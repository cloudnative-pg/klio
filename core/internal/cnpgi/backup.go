package cnpgi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/cloudnative-pg/cnpg-i/pkg/backup"
	"github.com/cloudnative-pg/machinery/pkg/log"
	pgTime "github.com/cloudnative-pg/machinery/pkg/postgres/time"
	"github.com/cloudnative-pg/machinery/pkg/types"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/common"
)

// backupServiceImplementation is the implementation
// of the Backup CNPG capability.
type backupServiceImplementation struct {
	backup.UnimplementedBackupServer

	InstanceName string
}

// GetCapabilities implements the Backup service interface.
func (b backupServiceImplementation) GetCapabilities(
	_ context.Context, _ *backup.BackupCapabilitiesRequest,
) (*backup.BackupCapabilitiesResult, error) {
	log.Info("receiving backup capabilities call")
	return &backup.BackupCapabilitiesResult{
		Capabilities: []*backup.BackupCapability{
			{
				Type: &backup.BackupCapability_Rpc{
					Rpc: &backup.BackupCapability_RPC{
						Type: backup.BackupCapability_RPC_TYPE_BACKUP,
					},
				},
			},
		},
	}, nil
}

// Backup implements the Backup interface.
func (b backupServiceImplementation) Backup(
	ctx context.Context,
	request *backup.BackupRequest,
) (*backup.BackupResult, error) {
	contextLogger := log.FromContext(ctx)

	// Step 1: get and apply the retention policies
	var cluster cnpgv1.Cluster
	if err := json.Unmarshal(request.GetClusterDefinition(), &cluster); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cluster definition: %w", err)
	}

	var cnpgBackup cnpgv1.Backup
	if err := json.Unmarshal(request.GetBackupDefinition(), &cnpgBackup); err != nil {
		return nil, fmt.Errorf("failed to unmarshal backup definition: %w", err)
	}

	r, err := extractRetentionFromConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to extract retention policy from configuration: %w", err)
	}

	if err = b.setRetentionPolicy(ctx, r); err != nil {
		// Yes this is intentional. If we don't set the retention policies from
		// the configuration file, it is not a major issue. We can continue with the backup.
		// The eventual error will be logged into the setRetentionPolicy function
		log.Error(err, "failed to set retention policy")
	}

	// Step 2: starting the backup
	backupName := fmt.Sprintf("backup-%v", pgTime.ToCompactISO8601(time.Now()))

	waitForWals := "--wait-for-wals=true"
	if cnpgBackup.Spec.Target == cnpgv1.BackupTargetStandby {
		waitForWals = "--wait-for-wals=false"
	}

	contextLogger.Info("Starting Klio backup", "backupName", backupName)
	//nolint:gosec
	cmd := exec.CommandContext(ctx,
		"klio",
		"backup",
		"run",
		"--config",
		backupRepositoryConfigPath,
		waitForWals,
		"-n",
		backupName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to execute klio backup run command: %w", err)
	}

	contextLogger.Info("Backup completed, getting metadata", "backupName", backupName)
	//nolint:gosec
	cmd = exec.CommandContext(
		ctx,
		"klio",
		"backup",
		"get-metadata",
		"--config",
		backupRepositoryConfigPath,
		backupName)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to execute klio backup get-metadata command: %w", err)
	}

	var metadata common.BackupMetadata
	if err := json.Unmarshal(stdout.Bytes(), &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse backup metadata: %w", err)
	}

	// Step 3: trigger maintenance
	contextLogger.Info("Starting Klio backup maintenance")
	cmd = exec.CommandContext(ctx, "klio", "backup", "maintenance", "--config", backupRepositoryConfigPath)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		contextLogger.Error(err, "failed to execute klio backup maintenance command, skipping")
	}

	return &backup.BackupResult{
		BackupName:        backupName,
		StartedAt:         metadata.StartedAt,
		StoppedAt:         metadata.StoppedAt,
		BackupLabelFile:   []byte(metadata.BackupLabel),
		TablespaceMapFile: []byte(metadata.TablespaceMap),
		Metadata:          metadata.Annotations,
		BeginLsn:          string(types.Int64ToLSN(metadata.StartLSN)),
		EndLsn:            string(types.Int64ToLSN(metadata.EndLSN)),
		BackupId:          backupName,
		InstanceId:        b.InstanceName,
		BeginWal:          metadata.StartWAL,
		EndWal:            metadata.EndWAL,
		Online:            true,
	}, nil
}

func (b backupServiceImplementation) setRetentionPolicy(ctx context.Context, r *Retention) error {
	contextLogger := log.FromContext(ctx)

	if r.IsEmpty() {
		contextLogger.Info("Skipping retention policy creation")
		return nil
	}

	klioArgs := []string{
		"retention", "set", "--config", backupRepositoryConfigPath,
	}
	if r.KeepAnnual != nil {
		klioArgs = append(klioArgs, "--keep-annual", strconv.Itoa(*r.KeepAnnual))
	}
	if r.KeepDaily != nil {
		klioArgs = append(klioArgs, "--keep-daily", strconv.Itoa(*r.KeepDaily))
	}
	if r.KeepHourly != nil {
		klioArgs = append(klioArgs, "--keep-hourly", strconv.Itoa(*r.KeepHourly))
	}
	if r.KeepLatest != nil {
		klioArgs = append(klioArgs, "--keep-latest", strconv.Itoa(*r.KeepLatest))
	}
	if r.KeepWeekly != nil {
		klioArgs = append(klioArgs, "--keep-weekly", strconv.Itoa(*r.KeepWeekly))
	}
	if r.KeepMonthly != nil {
		klioArgs = append(klioArgs, "--keep-monthly", strconv.Itoa(*r.KeepMonthly))
	}

	contextLogger.Info("Executing klio retention set", "args", klioArgs)
	cmd := exec.CommandContext(ctx, "klio", klioArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to execute 'klio retention set' command: %w", err)
	}

	contextLogger.Info("Executing klio retention get")
	cmd = exec.CommandContext(ctx, "klio", "retention", "get", "--config", backupRepositoryConfigPath)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to execute 'klio retention get' command: %w", err)
	}

	contextLogger.Info("Effective retention policy", "effectivePolicy", stdout.String())

	return nil
}
