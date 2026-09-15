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
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// checkPeerCluster verifies that the client certificate of the caller was
// issued for clusterName. The Common Name has the form userName@hostName,
// where the host part is the cluster the certificate grants access to.
func checkPeerCluster(ctx context.Context, clusterName string) error {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "no peer information")
	}

	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(tlsInfo.State.PeerCertificates) == 0 {
		return status.Error(codes.Unauthenticated, "no client certificate")
	}

	commonName := tlsInfo.State.PeerCertificates[0].Subject.CommonName
	_, certCluster, found := strings.Cut(commonName, "@")
	if !found || certCluster == "" {
		return status.Errorf(codes.PermissionDenied,
			"client certificate Common Name %q is not in the form userName@hostName", commonName)
	}

	if certCluster != clusterName {
		return status.Errorf(codes.PermissionDenied,
			"client certificate is issued for cluster %q, not %q", certCluster, clusterName)
	}

	return nil
}
