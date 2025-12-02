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

	"github.com/aws/aws-sdk-go/aws"             //nolint:staticcheck
	"github.com/aws/aws-sdk-go/aws/credentials" //nolint:staticcheck
	"github.com/aws/aws-sdk-go/aws/session"     //nolint:staticcheck
	s3 "github.com/fclairamb/afero-s3"
	"github.com/spf13/afero"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// CreateAWSSession creates an AWS session from the relative
// configuration.
func CreateAWSSession(ctx context.Context, cfg *config.Tier2Config) (*session.Session, error) {
	awsCfg := aws.Config{}

	if cfg.S3.Endpoint != "" {
		awsCfg.Endpoint = aws.String(cfg.S3.Endpoint)
		awsCfg.S3ForcePathStyle = aws.Bool(true)
	}
	if cfg.S3.Region != "" {
		awsCfg.Region = aws.String(cfg.S3.Region)
	}
	if cfg.S3.AccessKeyID != "" || cfg.S3.SecretAccessKey != "" || cfg.S3.SessionToken != "" {
		awsCfg.Credentials = credentials.NewStaticCredentials(
			cfg.S3.AccessKeyID,
			cfg.S3.SecretAccessKey,
			cfg.S3.SessionToken,
		)
	}
	if cfg.S3.CustomCABundleFile != "" {
		httpClient, err := newHTTPClient(ctx, cfg.S3.CustomCABundleFile)
		if err != nil {
			return nil, err
		}

		awsCfg.HTTPClient = httpClient
	}

	sess, err := session.NewSessionWithOptions(
		session.Options{
			Config: awsCfg,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("while creating an AWS session: %w", err)
	}

	return sess, err
}

// ConnectBase creates a FS abstraction to look into the file system where
// we allocated the Kopia tier 2 storage.
func ConnectBase(ctx context.Context, cfg *config.Tier2Config) (afero.Fs, error) {
	sess, err := CreateAWSSession(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return afero.NewBasePathFs(
		s3.NewFs(cfg.S3.BucketName, sess),
		path.Join(cfg.S3.Prefix, "base"),
	), nil
}

// ConnectWAL creates a FS abstraction to store WAL files over a tier 2 configuration.
func ConnectWAL(ctx context.Context, cfg *config.Tier2Config) (afero.Fs, error) {
	sess, err := CreateAWSSession(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return afero.NewBasePathFs(
		s3.NewFs(cfg.S3.BucketName, sess),
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
				MinVersion: tls.VersionTLS13,
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
