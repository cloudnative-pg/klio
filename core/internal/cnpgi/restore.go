package cnpgi

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/postgres"
	restore "github.com/cloudnative-pg/cnpg-i/pkg/restore/job"
)

// restoreImpl is the implementation of the restore job hooks.
type restoreImpl struct {
	restore.UnimplementedRestoreJobHooksServer

	PgDataPath string
}

// GetCapabilities returns the capabilities of the restore job hooks.
func (impl restoreImpl) GetCapabilities(
	_ context.Context,
	_ *restore.RestoreJobHooksCapabilitiesRequest,
) (*restore.RestoreJobHooksCapabilitiesResult, error) {
	return &restore.RestoreJobHooksCapabilitiesResult{
		Capabilities: []*restore.RestoreJobHooksCapability{
			{
				Kind: restore.RestoreJobHooksCapability_KIND_RESTORE,
			},
		},
	}, nil
}

// Restore restores the cluster from a backup.
func (impl restoreImpl) Restore(
	ctx context.Context,
	request *restore.RestoreRequest,
) (*restore.RestoreResponse, error) {
	var cluster cnpgv1.Cluster
	if err := json.Unmarshal(request.GetClusterDefinition(), &cluster); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cluster definition: %w", err)
	}
	configFile := "/var/lib/postgresql/klio/" + cluster.Spec.Bootstrap.Recovery.Source

	var backupID string
	var targetTime string

	if cluster.Spec.Bootstrap != nil && cluster.Spec.Bootstrap.Recovery != nil {
		recoveryConfiguration := cluster.Spec.Bootstrap.Recovery
		recoveryTarget := recoveryConfiguration.RecoveryTarget
		if recoveryTarget != nil {
			backupID = recoveryTarget.BackupID
			targetTime = recoveryTarget.TargetTime
		}
	}

	args := []string{
		"restore",
		"--config",
		configFile,
	}
	if backupID != "" {
		args = append(args, "--backup-id", backupID)
	}
	if targetTime != "" {
		args = append(args, "--target-time", targetTime)
	}
	args = append(args, impl.PgDataPath)

	cmd := exec.CommandContext(ctx, "klio", args...) //nolint:gosec
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to execute klio restore command: %w", err)
	}

	config := getRestoreWalConfig()

	return &restore.RestoreResponse{
		RestoreConfig: config,
		Envs:          []string{},
	}, nil
}

// getRestoreWalConfig obtains the content to append to `custom.conf` allowing PostgreSQL
// to complete the WAL recovery from the object storage and then start
// as a new primary.
func getRestoreWalConfig() string {
	restoreCmd := fmt.Sprintf(
		"/controller/manager wal-restore --log-destination %s/%s.json %%f %%p",
		postgres.LogPath, postgres.LogFileName)

	recoveryFileContents := fmt.Sprintf(
		"recovery_target_action = promote\n"+
			"restore_command = '%s'\n",
		restoreCmd)

	return recoveryFileContents
}
