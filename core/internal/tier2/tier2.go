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

package tier2

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	aferoS3 "github.com/fclairamb/afero-s3"
	"github.com/spf13/afero"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// CreateAWSS3Client creates an AWS S3 client configured from the tier2 configuration.
func CreateAWSS3Client(ctx context.Context, cfg *config.Tier2Config) (*s3.Client, error) {
	// Build configuration options
	var opts []func(*awsconfig.LoadOptions) error

	// Set region if provided
	if cfg.S3.Region != "" {
		opts = append(opts, awsconfig.WithRegion(cfg.S3.Region))
	}

	// Set credentials if provided
	if cfg.S3.AccessKeyID != "" || cfg.S3.SecretAccessKey != "" || cfg.S3.SessionToken != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				cfg.S3.AccessKeyID,
				cfg.S3.SecretAccessKey,
				cfg.S3.SessionToken,
			),
		))
	}

	// Set custom HTTP client with custom CA bundle if provided
	if cfg.S3.CustomCABundleFile != "" {
		httpClient, err := newHTTPClient(ctx, cfg.S3.CustomCABundleFile)
		if err != nil {
			return nil, err
		}
		opts = append(opts, awsconfig.WithHTTPClient(httpClient))
	}

	// Load AWS configuration
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("while loading AWS config: %w", err)
	}

	// Create S3 client options
	var s3Opts []func(*s3.Options)

	// Set custom endpoint and force path style if endpoint is defined
	if cfg.S3.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.S3.Endpoint)
			o.UsePathStyle = true
		})
	}

	s3Client := s3.NewFromConfig(awsCfg, s3Opts...)

	return s3Client, nil
}

// ConnectBase creates an FS abstraction to look into the file system where
// we allocated the Kopia tier 2 storage.
func ConnectBase(ctx context.Context, cfg *config.Tier2Config) (afero.Fs, error) {
	client, err := CreateAWSS3Client(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return afero.NewBasePathFs(
		aferoS3.NewFsFromClient(cfg.S3.BucketName, client),
		path.Join(cfg.S3.Prefix, "base"),
	), nil
}

// ConnectWAL creates an FS abstraction to store WAL files over a tier 2 configuration.
func ConnectWAL(ctx context.Context, cfg *config.Tier2Config) (afero.Fs, error) {
	client, err := CreateAWSS3Client(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return afero.NewBasePathFs(
		aferoS3.NewFsFromClient(cfg.S3.BucketName, client),
		path.Join(cfg.S3.Prefix, "wals"),
	), nil
}

func newHTTPClient(ctx context.Context, filename string) (*http.Client, error) {
	tlsCertPool, err := loadCustomCABundle(ctx, filename)
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

func loadCustomCABundle(ctx context.Context, filename string) (*x509.CertPool, error) {
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
