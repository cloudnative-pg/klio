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
	"fmt"

	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
)

// SendWALCoordinator adapts a *Connection to the sendwal.ReplicationCoordinator
// interface (github.com/cloudnative-pg/klio/core/pkg/sendwal), so the
// generic WAL receiver can negotiate streaming with a Klio server without
// that package knowing anything about Klio's gRPC protocol.
type SendWALCoordinator struct {
	conn        *Connection
	sendToTier2 bool
}

// NewSendWALCoordinator creates a new SendWALCoordinator.
func NewSendWALCoordinator(conn *Connection, sendToTier2 bool) *SendWALCoordinator {
	return &SendWALCoordinator{
		conn:        conn,
		sendToTier2: sendToTier2,
	}
}

// RequestStart implements sendwal.ReplicationCoordinator.
func (c *SendWALCoordinator) RequestStart(
	ctx context.Context,
	clusterName, systemID, currentWALName string,
) (string, error) {
	result, err := c.conn.RequestWALStart(ctx, &klioGRPC.RequestWALStartRequest{
		ClusterName:    clusterName,
		SystemId:       systemID,
		CurrentWalName: currentWALName,
	})
	if err != nil {
		return "", fmt.Errorf("while requesting WAL start: %w", err)
	}

	return result.GetWalName(), nil
}

// ResetStream implements sendwal.ReplicationCoordinator.
func (c *SendWALCoordinator) ResetStream(
	ctx context.Context,
	clusterName, systemID, currentWALName string,
) (string, error) {
	result, err := c.conn.ResetWALStream(ctx, &klioGRPC.ResetWALStreamRequest{
		ClusterName:    clusterName,
		SystemId:       systemID,
		CurrentWalName: currentWALName,
	})
	if err != nil {
		return "", fmt.Errorf("while resetting WAL stream: %w", err)
	}

	return result.GetWalName(), nil
}

// StoreHistoryFile implements sendwal.ReplicationCoordinator.
func (c *SendWALCoordinator) StoreHistoryFile(ctx context.Context, name string, content []byte) error {
	return c.conn.StoreHistoryFile(ctx, name, content, c.sendToTier2) //nolint:wrapcheck
}
