package cnpgi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

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
	_ *backup.BackupRequest,
) (*backup.BackupResult, error) {
	contextLogger := log.FromContext(ctx)

	backupName := fmt.Sprintf("backup-%v", pgTime.ToCompactISO8601(time.Now()))

	contextLogger.Info("Starting Klio backup", "backupName", backupName)
	//nolint:gosec
	cmd := exec.CommandContext(ctx,
		"klio",
		"backup",
		"run",
		"--config",
		backupRepositoryConfigPath,
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
