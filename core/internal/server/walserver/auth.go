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
	"errors"
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

var (
	errNoPeerInfo           = errors.New("no peer info in context")
	errNoTLSInfo            = errors.New("peer auth info is not TLS")
	errNoVerifiedClientCert = errors.New("no verified client certificate")
)

// peerClusterName extracts the cluster name from the Common Name of the
// client certificate presented over mTLS. The CN must be in the form
// userName@clusterName, matching ClientConfig.ClusterName on the client side.
func peerClusterName(ctx context.Context) (string, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return "", errNoPeerInfo
	}

	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return "", errNoTLSInfo
	}

	if len(tlsInfo.State.VerifiedChains) == 0 || len(tlsInfo.State.VerifiedChains[0]) == 0 {
		return "", errNoVerifiedClientCert
	}

	commonName := tlsInfo.State.VerifiedChains[0][0].Subject.CommonName

	_, clusterName, found := strings.Cut(commonName, "@")
	if !found || clusterName == "" {
		return "", fmt.Errorf("commonName must be in the form userName@hostName, got %q", commonName)
	}

	return clusterName, nil
}

// authorizeClusterName ensures the caller's client certificate was issued
// for clusterName. This prevents a client authenticated for one cluster
// from reading or writing another cluster's data by supplying a different
// cluster_name field in the request.
func authorizeClusterName(ctx context.Context, clusterName string) error {
	peerName, err := peerClusterName(ctx)
	if err != nil {
		return status.Errorf(codes.PermissionDenied, "cannot verify client certificate: %v", err)
	}

	if peerName != clusterName {
		return status.Errorf(codes.PermissionDenied, "client certificate is not authorized for cluster %q", clusterName)
	}

	return nil
}
