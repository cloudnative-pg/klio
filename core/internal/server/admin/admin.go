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

package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/nats-io/nats.go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/kopia"
	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
	kopiaWrapper "github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/internal/queue"
)

// Options contains configuration options for the admin server.
type Options struct {
	Tier1KopiaConfigFile string
	Tier2KopiaConfigFile string

	SocketPath string

	RunID                  string
	RunSecret              string
	CertificateFingerprint string

	Tier2ServerAddress string

	QueueURL string
}

// Server is the gRPC admin server implementation.
type Server struct {
	klioGRPC.UnimplementedAdminServer

	opts Options

	tier1KopiaClient *kopiaWrapper.Client
	tier2KopiaClient *kopiaWrapper.Client
	tier1            klioclient.Client
	tier2            klioclient.Client
	klio             klioclient.Client
	natsConn         *nats.Conn
	streamMgr        *queue.StreamManager
}

// New creates a new admin server instance with the provided options.
func New(opts Options) (*Server, error) {
	kopiaBinary, err := kopiaWrapper.LookupBinary()
	if err != nil {
		return nil, err
	}

	tier1, err := clientFromConfigFile(opts.Tier1KopiaConfigFile)
	if err != nil {
		return nil, err
	}

	tier2, err := clientFromConfigFile(opts.Tier2KopiaConfigFile)
	if err != nil {
		return nil, err
	}

	return &Server{
		tier1: tier1,
		tier2: tier2,
		klio: &kopia.MultiConnection{
			Tier1: tier1,
			Tier2: tier2,
		},
		tier1KopiaClient: &kopiaWrapper.Client{
			ConfigFile:  opts.Tier1KopiaConfigFile,
			KopiaBinary: kopiaBinary,
			Password:    "",
		},
		tier2KopiaClient: &kopiaWrapper.Client{
			ConfigFile:  opts.Tier2KopiaConfigFile,
			KopiaBinary: kopiaBinary,
			Password:    "",
		},
		opts: opts,
	}, nil
}

// Start starts the admin server and listens for gRPC connections.
func (s *Server) Start(ctx context.Context) error {
	contextLogger := log.FromContext(ctx)

	// Connect to NATS queue if QueueURL is provided
	if s.opts.QueueURL != "" {
		natsConn, err := nats.Connect(s.opts.QueueURL)
		if err != nil {
			return fmt.Errorf("while connecting to NATS: %w", err)
		}
		s.natsConn = natsConn

		// Ensure NATS connection is closed when Start() finishes
		defer func() {
			if s.natsConn != nil {
				s.natsConn.Close()
			}
		}()

		streamMgr, err := queue.NewStreamManager(natsConn)
		if err != nil {
			return fmt.Errorf("while creating queue stream manager: %w", err)
		}
		s.streamMgr = streamMgr
	}

	listener, err := s.createListener(ctx)
	if err != nil {
		return err
	}

	server := grpc.NewServer()
	klioGRPC.RegisterAdminServer(server, s)

	go func() {
		// Wait for context cancellation
		<-ctx.Done()

		// Trigger graceful shutdown
		server.GracefulStop()
	}()

	contextLogger.Info("Starting Klio administration server", "socket-path", s.opts.SocketPath)
	// Serve returns nil after a GracefulStop, and net.ErrClosed if the
	// listener is closed out from under it; neither is a real failure.
	if err := server.Serve(listener); err != nil && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("error while running server: %w", err)
	}

	return nil
}

func clientFromConfigFile(configFile string) (klioclient.Client, error) {
	if configFile == "" {
		return nil, nil
	}

	return kopia.FromKopiaConfig(configFile)
}

// ListBackups implements [grpc.AdminServer].
func (s *Server) ListBackups(
	ctx context.Context,
	_ *klioGRPC.ListBackupsRequest,
) (*klioGRPC.ListBackupsResult, error) {
	backupList, err := s.klio.ListBackups(ctx, "")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "while listing backups: %s", err.Error())
	}

	data, err := json.Marshal(backupList)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "while marshalling backup manifest: %s", err.Error())
	}

	return &klioGRPC.ListBackupsResult{
		BackupManifests: data,
	}, nil
}

// Refresh implements [grpc.AdminServer].
func (s *Server) Refresh(
	ctx context.Context,
	_ *klioGRPC.RefreshRequest,
) (*klioGRPC.RefreshResult, error) {
	if s.tier2KopiaClient != nil {
		if err := s.tier2KopiaClient.RefreshServer(ctx, kopiaWrapper.RefreshServerOptions{
			ServerControlUser:     s.opts.RunID,
			ServerControlPassword: s.opts.RunSecret,
			ServerCertFingerprint: s.opts.CertificateFingerprint,
			Address:               s.opts.Tier2ServerAddress,
		}); err != nil {
			return nil, status.Errorf(codes.Internal, "while refreshing tier2 server: %s", err.Error())
		}
	}

	return &klioGRPC.RefreshResult{}, nil
}

// QueueStatus implements [grpc.AdminServer].
func (s *Server) QueueStatus(
	_ context.Context,
	_ *klioGRPC.QueueStatusRequest,
) (*klioGRPC.QueueStatusResponse, error) {
	if s.streamMgr == nil {
		return nil, status.Errorf(
			codes.Unavailable,
			"queue status not available: server not configured with Stream Manager",
		)
	}

	// Get queue status
	queueStatus, err := s.streamMgr.GetStatus()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "while getting queue status: %s", err.Error())
	}

	return &klioGRPC.QueueStatusResponse{
		PendingBackups: queueStatus.PendingBackups,
		PendingWals:    queueStatus.PendingWALs,
	}, nil
}

// QueueListFailedBackups implements [grpc.AdminServer].
func (s *Server) QueueListFailedBackups(
	ctx context.Context,
	req *klioGRPC.QueueListFailedBackupsRequest,
) (*klioGRPC.QueueListFailedBackupsResponse, error) {
	if s.streamMgr == nil {
		return nil, status.Errorf(
			codes.Unavailable,
			"failed backups not available: server not configured with Stream Manager",
		)
	}

	opts := make([]queue.ListOption, 0)

	if name := req.GetClusterName(); name != "" {
		opts = append(opts, queue.WithCluster(name))
	}

	backupTasks, err := s.streamMgr.ListFailedBackupTasks(ctx, opts...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "while getting queue failed backups: %s", err.Error())
	}

	responseBackups := make([]*klioGRPC.FailedBackup, len(backupTasks))

	for i, task := range backupTasks {
		responseBackups[i] = &klioGRPC.FailedBackup{
			ClusterName:     task.Task.ClusterName,
			LastAttemptTime: timestamppb.New(task.Timestamp),
		}
	}

	return &klioGRPC.QueueListFailedBackupsResponse{
		Backups: responseBackups,
	}, nil
}

// QueueListFailedWALs implements [grpc.AdminServer].
func (s *Server) QueueListFailedWALs(
	ctx context.Context,
	req *klioGRPC.QueueListFailedWALsRequest,
) (*klioGRPC.QueueListFailedWALsResponse, error) {
	if s.streamMgr == nil {
		return nil, status.Errorf(
			codes.Unavailable,
			"failed WALs not available: server not configured with Stream Manager",
		)
	}
	opts := make([]queue.ListOption, 0)

	if name := req.GetClusterName(); name != "" {
		opts = append(opts, queue.WithCluster(name))
	}

	walTasks, err := s.streamMgr.ListFailedWALTasks(ctx, opts...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "while getting queue failed wal files: %s", err.Error())
	}

	responseWALs := make([]*klioGRPC.FailedWAL, len(walTasks))

	for i, task := range walTasks {
		responseWALs[i] = &klioGRPC.FailedWAL{
			ClusterName:     task.Task.ClusterName,
			WalName:         task.Task.WALName,
			Sequence:        task.Sequence,
			LastAttemptTime: timestamppb.New(task.Timestamp),
		}
	}

	return &klioGRPC.QueueListFailedWALsResponse{
		Wals: responseWALs,
	}, nil
}

// DeleteBackup implements [grpc.AdminServer].
func (s *Server) DeleteBackup(
	ctx context.Context,
	req *klioGRPC.DeleteBackupRequest,
) (*klioGRPC.DeleteBackupResponse, error) {
	if req.GetBackupName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "backup_name must be specified")
	}

	if req.GetClusterName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "cluster_name must be specified")
	}

	if len(req.GetTiers()) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "at least one tier must be specified")
	}

	for _, tier := range req.GetTiers() {
		if err := s.deleteBackupFromTier(ctx, tier, req.GetClusterName(), req.GetBackupName()); err != nil {
			return nil, err
		}
	}

	return &klioGRPC.DeleteBackupResponse{}, nil
}

func (s *Server) deleteBackupFromTier(
	ctx context.Context,
	tier klioGRPC.Tier,
	clusterName string,
	backupName string,
) error {
	switch tier {
	case klioGRPC.Tier_TIER_1:
		if s.tier1 == nil {
			return status.Errorf(codes.FailedPrecondition, "tier1 is not configured")
		}
		if err := s.tier1.DeleteBackup(ctx, clusterName, backupName); err != nil {
			return status.Errorf(codes.Internal, "while deleting backup from tier1: %s", err.Error())
		}

	case klioGRPC.Tier_TIER_2:
		if s.tier2 == nil {
			return status.Errorf(codes.FailedPrecondition, "tier2 is not configured")
		}
		if err := s.tier2.DeleteBackup(ctx, clusterName, backupName); err != nil {
			return status.Errorf(codes.Internal, "while deleting backup from tier2: %s", err.Error())
		}

	case klioGRPC.Tier_TIER_UNSPECIFIED:
		return status.Errorf(codes.InvalidArgument, "TIER_UNSPECIFIED is not a valid tier")

	default:
		return status.Errorf(codes.InvalidArgument, "unknown tier: %v", tier)
	}

	return nil
}

// createListener creates the Unix socket the admin server listens on, with
// restrictive permissions so only the user running the server can connect.
func (s *Server) createListener(ctx context.Context) (net.Listener, error) {
	// Remove any stale socket file
	if err := os.Remove(s.opts.SocketPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("while removing stale socket file: %w", err)
	}

	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "unix", s.opts.SocketPath)
	if err != nil {
		return nil, err
	}

	if err := os.Chmod(s.opts.SocketPath, 0o600); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("while setting socket permissions: %w", err)
	}

	return listener, nil
}
