package cnpgi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/cloudnative-pg/cnpg-i/pkg/wal"
	"github.com/cloudnative-pg/machinery/pkg/log"
)

type walServiceImplementation struct {
	enableDebug bool
	wal.UnimplementedWALServer
}

// GetCapabilities implements the WALService interface.
func (w walServiceImplementation) GetCapabilities(
	_ context.Context,
	_ *wal.WALCapabilitiesRequest,
) (*wal.WALCapabilitiesResult, error) {
	return &wal.WALCapabilitiesResult{
		Capabilities: []*wal.WALCapability{
			{
				Type: &wal.WALCapability_Rpc{
					Rpc: &wal.WALCapability_RPC{
						Type: wal.WALCapability_RPC_TYPE_RESTORE_WAL,
					},
				},
			},
		},
	}, nil
}

// Restore implements the WALService interface.
func (w walServiceImplementation) Restore(
	ctx context.Context,
	request *wal.WALRestoreRequest,
) (*wal.WALRestoreResult, error) {
	contextLogger := log.FromContext(ctx).WithName("wal_restore")
	walName := request.GetSourceWalName()
	destinationPath := request.GetDestinationFileName()

	contextLogger.Info("WAL restore operation started", "walName", walName, "destinationPath", destinationPath)

	if walName == "" || destinationPath == "" {
		contextLogger.Warning("WAL restore operation failed. WAL name and destination file name must be specified")
		return nil, errors.New("source WAL name and destination file name must be provided")
	}

	var debug string
	if w.enableDebug {
		debug = "--debug"
	}

	cmd := exec.CommandContext( //nolint: gosec
		ctx,
		"klio",
		"get-wal", walName, destinationPath, "--partial=true", debug)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to execute klio backup command: %w, stderr: %s", err, stderr.String())
	}

	return &wal.WALRestoreResult{}, nil
}
