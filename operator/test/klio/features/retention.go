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

package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	k8swait "k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/types"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
)

const (
	serverContainerName = "server"
	// klioPodSuffix is the suffix added to the server name to form the pod name.
	klioPodSuffix = "-klio-0"
	// tier1AnnotationName is the annotation key used to mark backups present in tier1.
	tier1AnnotationName = "klio.io/tier1"
	// tier2AnnotationName is the annotation key used to mark backups present in tier2.
	tier2AnnotationName = "klio.io/tier2"
	// presentAnnotationValue is the value set when a backup is present in a tier.
	presentAnnotationValue = "present"
	// rustfsContainerName is the container name in the RustFS pod.
	rustfsContainerName = "rustfs"
	// rustfsInstanceLabel is the label RustFS pods carry, keyed by the RustFS
	// resource name. RustFS runs as a Deployment, so its pod name is not
	// deterministic and must be resolved via this label.
	rustfsInstanceLabel = "app.kubernetes.io/instance"
)

// RetentionFeature defines a feature for testing combined tier1/tier2 backup
// and WAL retention.
type RetentionFeature struct {
	name            string
	setup           types.StepFunc
	teardown        types.StepFunc
	backups         []*cnpgv1.Backup
	klioServer      *kliov1alpha1.Server
	namespace       string
	tier1Keep       int32
	tier2Keep       int32
	backupTimeout   time.Duration
	convergeTimeout time.Duration
	checkInterval   time.Duration
	clusterName     string
	s3BucketName    string
	s3Prefix        string
	rustfsName      string
}

// RetentionFeatureConfig holds the configuration for creating a retention feature test.
type RetentionFeatureConfig struct {
	// Name of the retention feature test.
	Name string
	// Setup function to initialize test resources.
	Setup types.StepFunc
	// Teardown function to clean up test resources.
	Teardown types.StepFunc
	// Backups are the backup resources to be created.
	Backups []*cnpgv1.Backup
	// KlioServer is the Klio server resource.
	KlioServer *kliov1alpha1.Server
	// Namespace is the namespace where resources are created.
	Namespace string
	// Tier1Keep is the number of backups tier1 retention should keep.
	Tier1Keep int32
	// Tier2Keep is the number of backups tier2 retention should keep.
	Tier2Keep int32
	// BackupTimeout is the timeout for each backup to complete (defaults to 2 minutes).
	BackupTimeout time.Duration
	// ConvergeTimeout is the timeout for retention to converge after the last
	// backup completes (defaults to 5 minutes: tier1 pruning waits on tier2
	// catch-up via the sync-guard, so this must be longer than BackupTimeout).
	ConvergeTimeout time.Duration
	// CheckInterval is the interval for checking status (defaults to 10 seconds).
	CheckInterval time.Duration
	// ClusterName is the name of the CNPG cluster (used for WAL directory lookup).
	ClusterName string
	// S3BucketName is the RustFS bucket backing tier2.
	S3BucketName string
	// S3Prefix is the S3 prefix used for tier2 storage.
	S3Prefix string
	// RustFSName is the name of the RustFS Deployment backing tier2, used to
	// resolve its pod for on-disk WAL inspection.
	RustFSName string
}

// NewRetentionFeature creates a new RetentionFeature with the given configuration.
func NewRetentionFeature(config RetentionFeatureConfig) *RetentionFeature {
	if config.BackupTimeout <= 0 {
		config.BackupTimeout = 2 * time.Minute
	}
	if config.ConvergeTimeout <= 0 {
		config.ConvergeTimeout = 5 * time.Minute
	}
	if config.CheckInterval <= 0 {
		config.CheckInterval = 10 * time.Second
	}

	return &RetentionFeature{
		name:            config.Name,
		setup:           config.Setup,
		teardown:        config.Teardown,
		backups:         config.Backups,
		klioServer:      config.KlioServer,
		namespace:       config.Namespace,
		tier1Keep:       config.Tier1Keep,
		tier2Keep:       config.Tier2Keep,
		backupTimeout:   config.BackupTimeout,
		convergeTimeout: config.ConvergeTimeout,
		checkInterval:   config.CheckInterval,
		clusterName:     config.ClusterName,
		s3BucketName:    config.S3BucketName,
		s3Prefix:        config.S3Prefix,
		rustfsName:      config.RustFSName,
	}
}

// Name returns the name of the retention feature.
func (f *RetentionFeature) Name() string {
	return f.name
}

// Setup initializes the retention feature test.
func (f *RetentionFeature) Setup() types.StepFunc {
	return f.setup
}

// Teardown cleans up resources after the test is run.
func (f *RetentionFeature) Teardown() types.StepFunc {
	return f.teardown
}

// Run executes the retention feature test.
//
// The scenario takes one more backup than either tier's retention keeps
// (max(tier1Keep, tier2Keep) + 1, i.e. 3 backups for tier1Keep=1,
// tier2Keep=2), then asserts that both tiers converge to their expected
// backup count and that each tier's oldest remaining WAL segment is no
// older than what its surviving backups require.
func (f *RetentionFeature) Run() types.StepFunc {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()
		t.Log("Running combined tier1/tier2 retention feature test")

		r, err := resources.New(cfg.Client().RESTConfig())
		require.NoError(t, err, "failed to create resources client")

		var tier1FloorWAL, tier2FloorWAL string

		for i, backup := range f.backups {
			t.Logf("Creating backup %d/%d: %s", i+1, len(f.backups), backup.Name)
			require.NoError(t, r.Create(ctx, backup), "failed to create backup %s", backup.Name)

			err = wait.For(
				machineryConditions.BackupIsCompleted(r, backup),
				wait.WithTimeout(f.backupTimeout),
				wait.WithInterval(f.checkInterval),
			)
			require.NoError(t, err, "backup %s did not complete", backup.Name)
			require.NotEmpty(t, backup.Status.BeginWal, "backup %s has no begin WAL in its status", backup.Name)
			t.Logf("Backup %s completed, begin WAL %q", backup.Name, backup.Status.BeginWal)

			// The 2nd backup is the oldest survivor once tier2 prunes to
			// tier2Keep=2; the 3rd (last) backup is the sole survivor once
			// tier1 prunes to tier1Keep=1.
			switch i {
			case 1:
				tier2FloorWAL = backup.Status.BeginWal
			case len(f.backups) - 1:
				tier1FloorWAL = backup.Status.BeginWal
			}
		}

		klioPodName := f.klioServer.Name + klioPodSuffix

		t.Logf("Waiting for tier1 to converge to %d backup(s) and tier2 to %d backup(s)",
			f.tier1Keep, f.tier2Keep)
		err = wait.For(
			checkBackupCounts(r, f.namespace, f.klioServer.Name, f.tier1Keep, f.tier2Keep),
			wait.WithTimeout(f.convergeTimeout),
			wait.WithInterval(f.checkInterval),
		)
		require.NoError(t, err, "backup counts did not converge to tier1=%d tier2=%d", f.tier1Keep, f.tier2Keep)
		t.Log("Backup counts converged")

		t.Logf("Waiting for tier1 WAL floor to reach %q", tier1FloorWAL)
		var tier1Final []string
		err = wait.For(
			func(ctx context.Context) (bool, error) {
				walFiles, err := listTier1WALFiles(ctx, r, f.namespace, klioPodName, f.clusterName)
				if err != nil {
					return false, err
				}
				tier1Final = walFiles

				return len(walsOlderThan(walFiles, tier1FloorWAL)) == 0, nil
			},
			wait.WithTimeout(f.convergeTimeout),
			wait.WithInterval(f.checkInterval),
		)
		require.NoError(t, err, "tier1 WAL retention did not prune WALs older than %q; remaining: %v",
			tier1FloorWAL, walsOlderThan(tier1Final, tier1FloorWAL))
		require.NotEmpty(t, tier1Final, "no tier1 WAL files found for cluster %q after retention", f.clusterName)
		t.Log("Tier1 WAL floor verified")

		t.Logf("Waiting for tier2 WAL floor to reach %q", tier2FloorWAL)
		var tier2Final []string
		err = wait.For(
			func(ctx context.Context) (bool, error) {
				walFiles, err := listTier2WALFiles(ctx, r, f.namespace, f.rustfsName, f.s3BucketName, f.s3Prefix, f.clusterName)
				if err != nil {
					return false, err
				}
				tier2Final = walFiles

				return len(walsOlderThan(walFiles, tier2FloorWAL)) == 0, nil
			},
			wait.WithTimeout(f.convergeTimeout),
			wait.WithInterval(f.checkInterval),
		)
		require.NoError(t, err, "tier2 WAL retention did not prune WALs older than %q; remaining: %v",
			tier2FloorWAL, walsOlderThan(tier2Final, tier2FloorWAL))
		require.NotEmpty(t, tier2Final, "no tier2 WAL files found for cluster %q after retention", f.clusterName)
		t.Log("Tier2 WAL floor verified")

		t.Log("Retention test completed: backup counts and WAL floors verified on both tiers")

		return ctx
	}
}

// backupMetadata is the subset of `klio admin list-backups` JSON output this
// feature needs: the tier-presence annotations set by the multi-tier client
// (see core/internal/client/klioclient/kopia/multiconnect.go).
type backupMetadata struct {
	Annotations map[string]string `json:"annotations,omitempty"`
}

// checkBackupCounts checks whether tier1 and tier2 each have exactly the
// expected number of backups. Returns (false, nil) on transient errors to
// allow the wait to keep retrying.
func checkBackupCounts(
	r *resources.Resources,
	namespace string,
	serverName string,
	expectedTier1 int32,
	expectedTier2 int32,
) k8swait.ConditionWithContextFunc {
	return func(ctx context.Context) (bool, error) {
		podName := serverName + klioPodSuffix

		var stdout, stderr bytes.Buffer
		klioCmd := []string{"klio", "admin", "list-backups"}
		if err := r.ExecInPod(ctx, namespace, podName, serverContainerName, klioCmd, &stdout, &stderr); err != nil {
			return false, nil //nolint:nilerr
		}

		var backups []backupMetadata
		if err := json.Unmarshal(stdout.Bytes(), &backups); err != nil {
			return false, nil //nolint:nilerr
		}

		var tier1Count, tier2Count int32
		for _, b := range backups {
			if b.Annotations[tier1AnnotationName] == presentAnnotationValue {
				tier1Count++
			}
			if b.Annotations[tier2AnnotationName] == presentAnnotationValue {
				tier2Count++
			}
		}

		return tier1Count == expectedTier1 && tier2Count == expectedTier2, nil
	}
}

// listTier1WALFiles returns the WAL segment file names stored in tier1 for a
// cluster, sorted ascending. Partial files are excluded.
//
// WAL files live at /klio/data/wal/{clusterName}/{16-char-prefix}/{24-char-name}
// on the server pod; 'find' is unavailable in the minimal container, so we
// rely on shell globbing.
func listTier1WALFiles(
	ctx context.Context,
	r *resources.Resources,
	namespace, podName, clusterName string,
) ([]string, error) {
	var stdout, stderr bytes.Buffer
	listCmd := []string{
		"sh", "-c",
		fmt.Sprintf("ls /klio/data/wal/%s/*/0000* 2>/dev/null | sort", clusterName),
	}

	if err := r.ExecInPod(ctx, namespace, podName, serverContainerName, listCmd, &stdout, &stderr); err != nil {
		return nil, fmt.Errorf("failed to list tier1 WAL files in pod %q: %w", podName, err)
	}

	return walSegmentNames(stdout.String()), nil
}

// listTier2WALFiles returns the WAL segment file names stored in tier2 for a
// cluster, sorted ascending, read directly off RustFS's own on-disk bucket
// storage (not the server's local cache, which only mirrors what's been
// synced and could diverge from what's actually persisted).
//
// RustFS's on-disk format is not a flat key->file mapping: each object is
// stored as its own directory (named after the object's key) containing an
// xl.meta metadata file, e.g. .../wals/{clusterName}/{prefix}/{walFile}/xl.meta.
// So a plain `ls` on the glob descends into every matched WAL "file" and
// prints its xl.meta/part-UUID contents instead of the WAL name; -d lists
// the matched directory entries themselves, same as tier1's on-disk layout.
// RustFS runs as a Deployment, so its pod name is resolved via label rather
// than assumed.
func listTier2WALFiles(
	ctx context.Context,
	r *resources.Resources,
	namespace, rustfsName, bucketName, s3Prefix, clusterName string,
) ([]string, error) {
	podName, err := findPodByLabel(ctx, r, namespace, rustfsInstanceLabel+"="+rustfsName)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve RustFS pod: %w", err)
	}

	var stdout, stderr bytes.Buffer
	listCmd := []string{
		"sh", "-c",
		fmt.Sprintf("ls -d /data/%s/%s/wals/%s/*/0000* 2>/dev/null | sort", bucketName, s3Prefix, clusterName),
	}

	// ls exits non-zero when nothing matches, which is fine: we return an
	// empty list in that case.
	err = r.ExecInPod(ctx, namespace, podName, rustfsContainerName, listCmd, &stdout, &stderr)
	if err != nil {
		return nil, fmt.Errorf("failed to list tier2 WAL files in RustFS pod %q: %w", podName, err)
	}

	return walSegmentNames(stdout.String()), nil
}

// findPodByLabel returns the name of the (first) pod matching selector in
// namespace, or an error if none is found.
func findPodByLabel(ctx context.Context, r *resources.Resources, namespace, selector string) (string, error) {
	var pods corev1.PodList
	err := r.WithNamespace(namespace).List(ctx, &pods,
		resources.WithLabelSelector(selector),
	)
	if err != nil {
		return "", fmt.Errorf("failed to list pods matching %q in namespace %q: %w", selector, namespace, err)
	}
	if len(pods.Items) == 0 {
		return "", fmt.Errorf("%w: selector %q in namespace %q", errNoMatchingPod, selector, namespace)
	}

	return pods.Items[0].Name, nil
}

// errNoMatchingPod is returned by findPodByLabel when no pod matches.
var errNoMatchingPod = errors.New("no matching pod found")

// walSegmentNames extracts WAL segment basenames from `ls` output, one path
// per line, excluding partial files.
func walSegmentNames(lsOutput string) []string {
	output := strings.TrimSpace(lsOutput)
	if output == "" {
		return nil
	}

	var names []string
	for line := range strings.SplitSeq(output, "\n") {
		parts := strings.Split(line, "/")
		name := parts[len(parts)-1]
		if strings.HasSuffix(name, ".partial") {
			continue
		}
		names = append(names, name)
	}

	return names
}

// walsOlderThan returns the entries of walFiles that are strictly older than
// boundary. WAL segment names are fixed-width hex, so a lexicographic
// comparison matches WAL ordering.
func walsOlderThan(walFiles []string, boundary string) []string {
	var older []string
	for _, w := range walFiles {
		if w < boundary {
			older = append(older, w)
		}
	}

	return older
}
