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

package walserver

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

func peerContext(commonName string) func() context.Context {
	return func() context.Context {
		return peer.NewContext(context.Background(), &peer.Peer{
			AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{
				PeerCertificates: []*x509.Certificate{{Subject: pkix.Name{CommonName: commonName}}},
			}},
		})
	}
}

func TestCheckPeerCluster(t *testing.T) {
	tests := []struct {
		name     string
		ctx      func() context.Context
		cluster  string
		wantCode codes.Code
	}{
		{name: "matching cluster", ctx: peerContext("klio@cluster-a"), cluster: "cluster-a", wantCode: codes.OK},
		{
			name: "other cluster", ctx: peerContext("klio@cluster-a"), cluster: "cluster-b",
			wantCode: codes.PermissionDenied,
		},
		{name: "malformed CN", ctx: peerContext("cluster-a"), cluster: "cluster-a", wantCode: codes.PermissionDenied},
		{name: "empty host", ctx: peerContext("klio@"), cluster: "", wantCode: codes.PermissionDenied},
		{name: "no peer", ctx: context.Background, cluster: "cluster-a", wantCode: codes.Unauthenticated},
		{
			name: "no certificate", cluster: "cluster-a", wantCode: codes.Unauthenticated,
			ctx: func() context.Context {
				return peer.NewContext(context.Background(), &peer.Peer{AuthInfo: credentials.TLSInfo{}})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.Code(checkPeerCluster(tt.ctx(), tt.cluster)); got != tt.wantCode {
				t.Fatalf("checkPeerCluster() code = %v, want %v", got, tt.wantCode)
			}
		})
	}
}
