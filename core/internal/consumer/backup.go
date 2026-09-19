/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

package consumer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/cloudnative-pg/machinery/pkg/stringset"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	klioclientkopia "github.com/cloudnative-pg/klio/core/internal/client/klioclient/kopia"
	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
	"github.com/cloudnative-pg/klio/core/internal/queue"
)

// errTier2NotConfigured is returned when a backup requests a tier2 relay but
// the server has no tier2 configured. It fails the task (retried, then
// dead-lettered) so the misconfiguration is surfaced.
var errTier2NotConfigured = errors.New("backup requested tier2 relay but the server has no tier2 configured")

// backupSteps is implemented by *Backup in production and by a test stub in
// unit tests. It covers the five steps that processBackup orchestrates so
// that the orchestration logic can be exercised without real Kopia clients.
type backupSteps interface {
	listManifests(ctx context.Context, clusterName string) ([]kopia.Manifest, error)
	verifyTier1(ctx context.Context, clusterName string) error
	relayTier2(ctx context.Context, task *queue.BackupTask, entries []kopia.Manifest) error
}

// Backup represents a Backup consumer.
type Backup struct {
	opts         *BackupOptions
	tier1Kopia   *kopia.Client
	tier2Kopia   *kopia.Client
	tier1Client  *klioclientkopia.Connection
	tier2Client  *klioclientkopia.Connection
	tier2Enabled bool
	steps        backupSteps
}

// BackupOptions are the configuration of the WAL consumer.
type BackupOptions struct {
	// The queue to be used
	Queue *queue.Conn

	// A config file to connect to tier 1
	Tier1KopiaConfig string

	// A config file to connect to tier 2
	Tier2KopiaConfig string

	// The cache directory
	CacheDirectory string
}

// NewBackup creates a new Backup consumer.
func NewBackup(opts *BackupOptions) (*Backup, error) {
	kopiaBinary, err := kopia.LookupBinary()
	if err != nil {
		return nil, err
	}

	tier1Client, err := klioclientkopia.FromKopiaConfig(opts.Tier1KopiaConfig)
	if err != nil {
		return nil, fmt.Errorf("while creating tier1 client: %w", err)
	}

	b := &Backup{
		opts: opts,
		tier1Kopia: &kopia.Client{
			KopiaBinary: kopiaBinary,
			ConfigFile:  opts.Tier1KopiaConfig,
		},
		tier1Client: tier1Client,
	}

	// The tier2 clients are only created when tier2 is configured. Without
	// them the consumer only performs tier1 maintenance.
	if opts.Tier2KopiaConfig != "" {
		tier2Client, err := klioclientkopia.FromKopiaConfig(opts.Tier2KopiaConfig)
		if err != nil {
			return nil, fmt.Errorf("while creating tier2 client: %w", err)
		}

		b.tier2Kopia = &kopia.Client{
			KopiaBinary: kopiaBinary,
			ConfigFile:  opts.Tier2KopiaConfig,
		}
		b.tier2Client = tier2Client
		b.tier2Enabled = true
	}

	b.steps = b

	return b, nil
}

// Run starts the consumer until the context is canceled or the
// SIGINT signal arrives.
func (d *Backup) Run(ctx context.Context) error {
	consumerCtx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	return d.opts.Queue.ConsumeBackupReceivedMessages(consumerCtx, d.processBackup)
}

// processBackup performs the post-backup work for a task and records a
// per-operation metric for the relay and maintenance stages. A returned error
// means the task should be retried (and, once MaxDeliver is exhausted,
// dead-lettered).
func (d *Backup) processBackup(ctx context.Context, task *queue.BackupTask) error {
	contextLogger := log.FromContext(ctx)
	contextLogger.Info("Processing backup", "task", task)

	entries, err := d.steps.listManifests(ctx, task.ClusterName)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		return nil
	}

	// Verify tier1 backups (catches any issues since sidecar verification).
	// We verify all backups for the cluster rather than a specific one because
	// MigrateSnapshots works at the source level (cluster), not individual backups.
	// The BackupTask doesn't include the backup name for this reason.
	if err := d.steps.verifyTier1(ctx, task.ClusterName); err != nil {
		return err
	}

	return d.relayToTier2IfRequested(ctx, task, entries)
}

// relayToTier2IfRequested migrates a verified backup to tier2 when
// requested. Backup retention (tier1 and tier2) and WAL retention are
// handled independently by the periodic retention sweeper
// (internal/retention), not here.
func (d *Backup) relayToTier2IfRequested(ctx context.Context, task *queue.BackupTask, entries []kopia.Manifest) error {
	contextLogger := log.FromContext(ctx)

	// A tier2 relay requested against a server with no tier2 is a
	// misconfiguration: record it as a relay failure and fail the task so it
	// is retried and eventually dead-lettered, surfacing the gap.
	if task.SendToTier2 && !d.tier2Enabled {
		contextLogger.Error(nil,
			"Backup requested tier2 relay but the server has no tier2 configured",
			"cluster", task.ClusterName)
		recordRelay(ctx, task.ClusterName, errTier2NotConfigured)

		return errTier2NotConfigured
	}

	if !task.SendToTier2 {
		return nil
	}

	relayErr := d.steps.relayTier2(ctx, task, entries)
	recordRelay(ctx, task.ClusterName, relayErr)

	return relayErr
}

// relayTier2 migrates the cluster's backups to tier2 and verifies them there.
func (d *Backup) relayTier2(ctx context.Context, task *queue.BackupTask, entries []kopia.Manifest) error {
	sources := manifestListToDescriptors(entries)

	// Set the per-cluster tier2 compression policy before migrating so that
	// the data relayed to tier2 is compressed. This overrides the tier2
	// repository global policy for this cluster's source. Applied even when
	// the policy is zero, so removing the compression section resets the
	// source back to inheriting the global policy instead of leaving a stale
	// override in place. A direct write is unavoidable here (the consumer has
	// no tier2 server connection) and is safe: it only writes a policy
	// manifest.
	if p := task.Tier2CompressionPolicy; p != nil && len(entries) > 0 {
		target := kopia.Target{
			Username: entries[0].Source.UserName,
			Hostname: task.ClusterName,
		}
		if err := d.tier2Kopia.SetKopiaCompressionPolicy(ctx, target, *p); err != nil {
			return fmt.Errorf("while setting the tier2 compression policy: %w", err)
		}
	}

	if err := d.tier2Kopia.MigrateSnapshots(ctx, kopia.SnapshotMigrateOpts{
		SourceConfig: d.opts.Tier1KopiaConfig,
		Sources:      sources,
		Tags: []string{
			klioclient.TablespaceNameTagName,
			klioclient.BackupContentTagName,
			klioclient.BackupNameTagName,
		},
	}); err != nil {
		return err
	}

	// Verify tier2 backups after migration
	return d.verifyTier2Backups(ctx, task.ClusterName)
}

func (d *Backup) listManifests(ctx context.Context, cluster string) ([]kopia.Manifest, error) {
	contextLogger := log.FromContext(ctx)
	entries, err := d.tier1Kopia.ListSnapshots(ctx, nil, contextLogger.Info)
	if err != nil {
		return nil, fmt.Errorf("while executing Kopia command: %w", err)
	}

	result := make([]kopia.Manifest, 0, len(entries))
	for i := range entries {
		if entries[i].Source.Host != cluster {
			continue
		}

		result = append(result, entries[i])
	}

	return result, nil
}

func manifestListToDescriptors(entries []kopia.Manifest) []string {
	result := stringset.New()
	for _, entry := range entries {
		result.Put(entry.Source.String())
	}

	return result.ToSortedList()
}

func (d *Backup) verifyTier1(ctx context.Context, clusterName string) error {
	contextLogger := log.FromContext(ctx)
	contextLogger.Info("Verifying tier1 backups", "cluster", clusterName)

	err := d.tier1Client.VerifyBackups(ctx, klioclientkopia.VerifyOpts{
		Hostname: clusterName,
		All:      true,
	})
	if err != nil {
		if _, ok := errors.AsType[*klioclientkopia.BackupVerificationError](err); ok {
			recordVerificationFailure(ctx, opentelemetry.Tier1)

			return fmt.Errorf("tier1 verification detected corruption: %w", err)
		}
		contextLogger.Error(err, "Tier1 verification encountered infrastructure error")

		return err
	}

	recordVerificationSuccess(ctx, opentelemetry.Tier1)

	return nil
}

func (d *Backup) verifyTier2Backups(ctx context.Context, clusterName string) error {
	contextLogger := log.FromContext(ctx)
	contextLogger.Info("Verifying tier2 backups after migration", "cluster", clusterName)

	err := d.tier2Client.VerifyBackups(ctx, klioclientkopia.VerifyOpts{
		Hostname: clusterName,
		All:      true,
	})
	if err != nil {
		if _, ok := errors.AsType[*klioclientkopia.BackupVerificationError](err); ok {
			recordVerificationFailure(ctx, opentelemetry.Tier2)

			return fmt.Errorf("tier2 verification detected corruption: %w", err)
		}
		contextLogger.Error(err, "Tier2 verification encountered infrastructure error, continuing")
	} else {
		recordVerificationSuccess(ctx, opentelemetry.Tier2)
	}

	return nil
}
