package walserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/cloudnative-pg/machinery/pkg/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/internal/queue"
)

// CloseBackup implements the CloseBackup GRPC call.
func (w *Implementation) CloseBackup(
	ctx context.Context,
	request *grpc.CloseBackupRequest,
) (*grpc.CloseBackupResult, error) {
	// Step 1: verify if the WALs have been archived
	missingWALFiles, err := w.checkWALFiles(request)
	if err != nil {
		return nil, err
	}

	if len(missingWALFiles) > 0 {
		return &grpc.CloseBackupResult{
			Tier2Schedule:   false,
			MissingWalFiles: missingWALFiles,
		}, nil
	}

	// Step 2: notify the queue of the completed backup
	if request.GetSendToTier2() {
		if err := w.scheduleBackupRelay(ctx, request); err != nil {
			return nil, err
		}
	}

	return &grpc.CloseBackupResult{
		Tier2Schedule:   w.queue != nil,
		MissingWalFiles: nil,
	}, nil
}

func (w *Implementation) scheduleBackupRelay(ctx context.Context, request *grpc.CloseBackupRequest) error {
	contextLogger := log.FromContext(ctx)

	if w.queue == nil {
		return status.Errorf(
			codes.Internal,
			"queue service is uninitialized",
		)
	}

	var tier2Policy *kopia.RetentionPolicy
	if request.GetTier2RetentionPolicy() != "" {
		var policy kopia.RetentionPolicy
		if err := json.Unmarshal([]byte(request.GetTier2RetentionPolicy()), &policy); err != nil {
			contextLogger.Error(err, "Unable to unmarshal tier2 retention policy, skipping")
		} else {
			tier2Policy = &policy
		}
	}

	if err := w.queue.NotifyBackupReceived(ctx, &queue.BackupTask{
		ClusterName:          request.GetClusterName(),
		Tier2RetentionPolicy: tier2Policy,
	}); err != nil {
		return fmt.Errorf("while sending task to queue: %w", err)
	}

	return nil
}

func (w *Implementation) checkWALFiles(request *grpc.CloseBackupRequest) ([]string, error) {
	startLSN, err := types.LSNStartFromWALName(request.GetStartWal(), request.GetSegmentSize())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid start_wal: %q", request.GetStartWal())
	}

	startPos, err := startLSN.Parse()
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid start_wal: %q (%q)", request.GetStartWal(), startLSN)
	}

	endLSN, err := types.LSNStartFromWALName(request.GetEndWal(), request.GetSegmentSize())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid LSN start from end_wal: %q", request.GetEndWal())
	}

	endPos, err := endLSN.Parse()
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid end_wal: %q (%q)", request.GetEndWal(), startLSN)
	}

	var missingWALFiles []string
	for pos := startPos; pos <= endPos; pos += request.GetSegmentSize() {
		if pos > endPos {
			break
		}

		name, err := types.Int64ToLSN(pos).WALFileName(int(request.GetTimeline()), request.GetSegmentSize())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "Invalid WAL position: (%q)", pos)
		}

		exists, err := w.conn.IsWALFileExisting(request.GetClusterName(), name)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "While checking existence of WAL file %q: %v", name, err.Error())
		}

		if !exists {
			missingWALFiles = append(missingWALFiles, name)
		}
	}

	return missingWALFiles, nil
}
