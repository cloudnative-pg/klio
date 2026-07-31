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
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// connectToAdminServer establishes a gRPC connection to the Klio admin server
// via a Unix socket.
//
// We use insecure credentials because:
//   - Unix sockets provide local IPC only (not exposed to the network)
//   - Security is enforced through file system permissions on the socket
//   - The server sets restrictive permissions (0600) on the socket file at creation
//   - TLS adds no additional security value for local Unix socket communication
//
// The caller must close the returned connection.
func connectToAdminServer(socketPath string) (*grpc.ClientConn, error) {
	conn, err := grpc.NewClient(
		"unix:"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("while establishing connection to the server: %w", err)
	}

	return conn, nil
}
