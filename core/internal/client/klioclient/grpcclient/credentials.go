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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"

	"google.golang.org/grpc/credentials"

	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// loadClientIdentity loads the client key pair referenced by the client
// configuration, reading the files fresh on every call. It backs the
// GetClientCertificate callback, so a rotated client identity is
// presented on new handshakes without restarting the process.
func loadClientIdentity(clientConfig *config.ClientConfig) (tls.Certificate, error) {
	clientCertificate, err := tls.LoadX509KeyPair(clientConfig.Wal.ClientCertPath, clientConfig.Wal.ClientKeyPath)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("while parsing the client certificate: %w", err)
	}

	return clientCertificate, nil
}

// loadServerCABundle reads the PEM-encoded CA bundle used to verify the
// Klio server. Callers re-read it whenever they need it so a rotated
// bundle takes effect without restarting the process.
func loadServerCABundle(bundlePath string) (*x509.CertPool, error) {
	certPEMBlock, err := os.ReadFile(bundlePath) //nolint:gosec // path comes from validated config
	if err != nil {
		return nil, fmt.Errorf("while reading the server CA bundle: %w", err)
	}

	serverCertificatePool := x509.NewCertPool()
	if !serverCertificatePool.AppendCertsFromPEM(certPEMBlock) {
		return nil, ErrInconsistentCertificate
	}

	return serverCertificatePool, nil
}

// buildTLSConfig builds the client TLS configuration from the current
// trust bundle on disk, wiring the client identity for per-handshake
// reloads. The eager identity load fails fast on invalid files.
func buildTLSConfig(clientConfig *config.ClientConfig) (*tls.Config, error) {
	serverCertificatePool, err := loadServerCABundle(clientConfig.Wal.ServerCertPath)
	if err != nil {
		return nil, err
	}

	if _, err := loadClientIdentity(clientConfig); err != nil {
		return nil, err
	}

	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    serverCertificatePool,
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			clientCertificate, err := loadClientIdentity(clientConfig)
			if err != nil {
				return nil, err
			}

			return &clientCertificate, nil
		},
	}, nil
}

// reloadingCredentials builds a fresh TLS configuration on every client
// handshake, so a rotated trust bundle or client identity takes effect on
// reconnects without restarting the process, while certificate
// verification stays with crypto/tls. A reload failure fails that
// handshake closed.
type reloadingCredentials struct {
	credentials.TransportCredentials

	clientConfig *config.ClientConfig
}

// ClientHandshake implements credentials.TransportCredentials.
//
//nolint:ireturn
func (c *reloadingCredentials) ClientHandshake(
	ctx context.Context, authority string, rawConn net.Conn,
) (net.Conn, credentials.AuthInfo, error) {
	tlsConfig, err := buildTLSConfig(c.clientConfig)
	if err != nil {
		return nil, nil, err
	}

	return credentials.NewTLS(tlsConfig).ClientHandshake(ctx, authority, rawConn)
}

// Clone implements credentials.TransportCredentials.
//
//nolint:ireturn
func (c *reloadingCredentials) Clone() credentials.TransportCredentials {
	return &reloadingCredentials{
		TransportCredentials: c.TransportCredentials.Clone(),
		clientConfig:         c.clientConfig,
	}
}

// newReloadingCredentials loads the credentials once to fail fast on
// invalid files at connection setup.
func newReloadingCredentials(clientConfig *config.ClientConfig) (*reloadingCredentials, error) {
	tlsConfig, err := buildTLSConfig(clientConfig)
	if err != nil {
		return nil, err
	}

	return &reloadingCredentials{
		TransportCredentials: credentials.NewTLS(tlsConfig),
		clientConfig:         clientConfig,
	}, nil
}
