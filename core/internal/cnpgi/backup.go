package cnpgi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/cloudnative-pg/cnpg-i/pkg/backup"
	"github.com/cloudnative-pg/machinery/pkg/log"
	pgTime "github.com/cloudnative-pg/machinery/pkg/postgres/time"
	"github.com/cloudnative-pg/machinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/common"
)

// BackupServiceImplementation is the implementation
// of the Backup CNPG capability.
type BackupServiceImplementation struct {
	Client       client.Client
	InstanceName string
	backup.UnimplementedBackupServer
}

// GetCapabilities implements the Backup service interface.
func (b BackupServiceImplementation) GetCapabilities(
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
func (b BackupServiceImplementation) Backup(
	ctx context.Context,
	_ *backup.BackupRequest,
) (*backup.BackupResult, error) {
	contextLogger := log.FromContext(ctx)

	backupName := fmt.Sprintf("backup-%v", pgTime.ToCompactISO8601(time.Now()))

	cmd := exec.CommandContext(ctx, "klio", "--config=/config/klio.yaml", "backup", "-n", backupName) //nolint:gosec

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to execute klio backup command: %w, stderr: %s", err, stderr.String())
	}
	output := stdout.String()
	var metadata common.BackupMetadata

	contextLogger.Info("receiving backup metadata call", "metadata", output)

	// get last line

	lines := bytes.Split([]byte(output), []byte("\n"))
	if len(lines) == 0 {
		return nil, errors.New("no output received from klio backup command")
	}
	output = string(lines[len(lines)-1])
	contextLogger.Info("parsing backup metadata", "output", output)

	if err := json.Unmarshal([]byte(output), &metadata); err != nil {
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
