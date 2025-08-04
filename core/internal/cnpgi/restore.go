package cnpgi

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/postgres"
	restore "github.com/cloudnative-pg/cnpg-i/pkg/restore/job"
)

// restoreImpl is the implementation of the restore job hooks.
type restoreImpl struct {
	restore.UnimplementedRestoreJobHooksServer
	PgDataPath string
	BackupName string
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
	_ *restore.RestoreRequest,
) (*restore.RestoreResponse, error) {
	cmd := exec.CommandContext( //nolint: gosec
		ctx,
		"klio",
		"restore", impl.BackupName, impl.PgDataPath)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to execute klio restore command: %w, stderr: %s", err, stderr.String())
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
