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

package awscreds

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

// NewHTTPClientWithCABundle builds an HTTP client whose TLS trust store is
// the CA bundle read from the given PEM file, for use by AWS SDK clients
// talking to S3 endpoints signed by a private CA.
func NewHTTPClientWithCABundle(ctx context.Context, caBundleFile string) (*http.Client, error) {
	tlsCertPool, err := loadCABundle(ctx, caBundleFile)
	if err != nil {
		return nil, err
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    tlsCertPool,
				MinVersion: tls.VersionTLS12,
			},
		},
	}, nil
}

func loadCABundle(ctx context.Context, filename string) (*x509.CertPool, error) {
	logger := log.FromContext(ctx)

	f, err := os.Open(filename) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("failed to open custom CA bundle PEM file, %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			logger.Error(err, "while closing custom CA bundle PEM file", "filename", filename)
		}
	}()

	return loadCertPool(f)
}

func loadCertPool(r io.Reader) (*x509.CertPool, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read custom CA bundle PEM file, %w", err)
	}

	p := x509.NewCertPool()
	if !p.AppendCertsFromPEM(b) {
		return nil, errors.New("failed to load custom CA bundle PEM file")
	}

	return p, nil
}
