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

package grpcclient

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/repository"
	"github.com/cloudnative-pg/klio/core/internal/server/walserver"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// TemporaryConnection is a connection to a temporary repository, to be
// deleted after the client is closed.
type TemporaryConnection struct {
	Connection

	listener net.Listener
}

// ConnectTemporary creates a connection to a local Kopia repository, creating it
// if not initialized.
func ConnectTemporary(
	ctx context.Context,
	logger log.Logger,
	clientConfig *config.ClientConfig,
	opts repository.Options,
) (*TemporaryConnection, error) {
	//nolint:gosec
	listeningPort := rand.IntN(1000) + 5000
	address := fmt.Sprintf("localhost:%v", listeningPort)

	if err := repository.Initialize(opts); err != nil {
		return nil, fmt.Errorf("initializing repository: %w", err)
	}

	repoConnection, err := repository.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("cannot open local repository: %w", err)
	}

	lc := &net.ListenConfig{}
	listener, err := lc.Listen(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("cannot listen on TCP socket: %w", err)
	}

	server := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
	klioGRPC.RegisterWALServer(
		server,
		walserver.New(
			walserver.Options{
				Connection: repoConnection,
				ReadOnly:   false,
			},
		),
	)

	go func() {
		if err := server.Serve(listener); !errors.Is(err, net.ErrClosed) {
			logger.Error(err, "error while running temporary server")
		}
	}()

	conn, err := grpc.NewClient(
		address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("while establishing connection to the server: %w", err)
	}

	walClient := klioGRPC.NewWALClient(conn)

	return &TemporaryConnection{
		Connection: Connection{
			clientConfig:   clientConfig,
			WALClient:      walClient,
			grpcConnection: conn,
		},
		listener: listener,
	}, nil
}

// Close closes the connection to the repository.
func (s *TemporaryConnection) Close() error {
	if err := s.listener.Close(); err != nil {
		return fmt.Errorf("while closing listener: %w", err)
	}

	if err := s.grpcConnection.Close(); err != nil {
		return fmt.Errorf("while closing connection: %w", err)
	}

	return nil
}
