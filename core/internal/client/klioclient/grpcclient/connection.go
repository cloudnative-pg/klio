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
	"os"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/wal"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// Connection represents a connection to a Klio server.
type Connection struct {
	klioGRPC.WALClient

	clientConfig   *config.ClientConfig
	grpcConnection *grpc.ClientConn
}

// StoreWALStreaming implements the WAL streaming service.
//
//nolint:ireturn
func (c *Connection) StoreWALStreaming(
	ctx context.Context,
	name string,
	segmentSize uint64,
	sendToTier2 bool,
	walStartLSN uint64,
	feedbackChannel chan<- wal.Feedback,
) (klioclient.WALUploader, error) {
	stream, err := c.Put(ctx)
	if err != nil {
		return nil, fmt.Errorf("while starting uploading a WAL file: %w", err)
	}

	g := &grpcWALStream{
		innerStream:     stream,
		segmentSize:     segmentSize,
		clusterName:     c.clientConfig.ClusterName,
		walName:         name,
		sendToTier2:     sendToTier2,
		walStartLSN:     walStartLSN,
		feedbackChannel: feedbackChannel,
	}
	g.startFeedbackReader()

	return g, nil
}

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

// verifyServerPeerCertificates verifies the presented server certificates
// against the CA bundle file, reading the file fresh on every call. It backs
// the VerifyConnection callback, which runs on every handshake including
// resumed sessions, so a rotated server CA is trusted on new handshakes
// (and reconnects after a failure) without restarting the process.
func verifyServerPeerCertificates(
	serverName, bundlePath string,
	peerCertificates []*x509.Certificate,
) error {
	// An empty DNSName would make x509 skip the hostname check.
	if serverName == "" {
		return ErrNoServerName
	}

	if len(peerCertificates) == 0 {
		return ErrNoServerCertificate
	}

	serverCertificatePool, err := loadServerCABundle(bundlePath)
	if err != nil {
		return err
	}

	intermediates := x509.NewCertPool()
	for _, intermediate := range peerCertificates[1:] {
		intermediates.AddCert(intermediate)
	}

	if _, err := peerCertificates[0].Verify(x509.VerifyOptions{
		DNSName:       serverName,
		Roots:         serverCertificatePool,
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}); err != nil {
		return fmt.Errorf("while verifying the server certificate: %w", err)
	}

	return nil
}

// buildTLSConfig builds the client TLS configuration, reading the server
// trust bundle on every handshake and wiring the client identity for
// per-handshake reloads. The eager loads fail fast on invalid files.
func buildTLSConfig(clientConfig *config.ClientConfig) (*tls.Config, error) {
	// Load once to fail fast on invalid files at connection setup.
	// Afterwards both the trust bundle and the identity are re-read on
	// every handshake, so rotated credentials take effect on reconnects
	// without restarting the process. A reload failure fails that
	// handshake closed.
	if _, err := loadServerCABundle(clientConfig.Wal.ServerCertPath); err != nil {
		return nil, err
	}

	if _, err := loadClientIdentity(clientConfig); err != nil {
		return nil, err
	}

	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		//nolint:gosec // G402: stock verification is skipped so the trust
		// bundle can be reloaded from disk on every handshake instead of
		// once at startup; VerifyConnection below still performs full
		// chain and hostname verification against the fresh bundle.
		InsecureSkipVerify: true,
		VerifyConnection: func(state tls.ConnectionState) error {
			return verifyServerPeerCertificates(
				state.ServerName, clientConfig.Wal.ServerCertPath, state.PeerCertificates)
		},
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			clientCertificate, err := loadClientIdentity(clientConfig)
			if err != nil {
				return nil, err
			}

			return &clientCertificate, nil
		},
	}, nil
}

// Connect opens a connection to a Klio server.
func Connect(clientConfig *config.ClientConfig, address string) (*Connection, error) {
	tlsConfig, err := buildTLSConfig(clientConfig)
	if err != nil {
		return nil, err
	}

	conn, err := grpc.NewClient(
		address,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
		grpc.WithInitialWindowSize(wal.GRPCInitialWindowSizeBytes),
		grpc.WithInitialConnWindowSize(wal.GRPCInitialConnWindowSizeBytes),
		grpc.WithReadBufferSize(wal.GRPCSocketBufferSizeBytes),
		grpc.WithWriteBufferSize(wal.GRPCSocketBufferSizeBytes),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return nil, fmt.Errorf("while establishing connection to the server: %w", err)
	}

	walClient := klioGRPC.NewWALClient(conn)

	return &Connection{
		clientConfig:   clientConfig,
		WALClient:      walClient,
		grpcConnection: conn,
	}, nil
}
