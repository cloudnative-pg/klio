package kopia

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"path"
	"strings"

	"github.com/cloudnative-pg/machinery/pkg/log"
)

// S3RepoOpts contains options for S3 repository operations.
type S3RepoOpts struct {
	CommonRepoOpts

	// BucketName is the name of the S3 bucket where the repository is stored.
	BucketName string

	// Endpoint is the S3 endpoint URL (e.g., for MinIO or custom S3-compatible storage).
	Endpoint string

	// Region is the AWS region where the S3 bucket is located.
	Region string

	// Prefix is the path prefix within the bucket where repository data is stored.
	Prefix string

	// AccessKeyID is the AWS access key ID for authentication.
	AccessKeyID string

	// SecretAccessKey is the AWS secret access key for authentication.
	SecretAccessKey string

	// SessionToken is the AWS session token for temporary credentials.
	SessionToken string

	// CustomCABundleFile is the path to a custom CA certificate bundle for TLS verification.
	CustomCABundleFile string
}

// InitializeS3 initializes a new Kopia repository on S3.
func InitializeS3(ctx context.Context, cfg S3RepoOpts) error {
	contextLogger := log.FromContext(ctx)

	args := make([]string, 0, 3)
	args = append(args,
		"repository", "create", "s3",
		"--create-only",
	)

	backendArgs, err := getCommonS3Args(cfg)
	if err != nil {
		return err
	}

	args = append(args, backendArgs...)

	kopiaRepositoryInitialize := exec.CommandContext(ctx, cfg.KopiaBinary, args...) //nolint:gosec
	kopiaRepositoryInitialize.Env = append(kopiaRepositoryInitialize.Env, getCommonS3Env(cfg)...)

	contextLogger.Info("Kopia repository initialize", "args", kopiaRepositoryInitialize.Args)
	if err := RunWithLogCapture(ctx, kopiaRepositoryInitialize, nil); err != nil {
		return fmt.Errorf("while creating Kopia repository: %w", err)
	}

	return nil
}

// ConnectS3 connects to an existing Kopia repository on S3.
func ConnectS3(ctx context.Context, configFileName string, opts S3RepoOpts) error {
	contextLogger := log.FromContext(ctx)

	args := []string{
		"repository", "connect", "s3",
		"--config-file=" + configFileName,
		"--override-username=klio",
		"--override-hostname=klio",
	}

	if opts.PersistCredentials {
		args = append(args, "--persist-credentials")
	}

	backendArgs, err := getCommonS3Args(opts)
	if err != nil {
		return err
	}

	args = append(args, backendArgs...)

	kopiaRepositoryConnect := exec.CommandContext(ctx, opts.KopiaBinary, args...) //nolint:gosec
	kopiaRepositoryConnect.Env = append(kopiaRepositoryConnect.Env, getCommonS3Env(opts)...)

	contextLogger.Info("Kopia repository connect", "args", kopiaRepositoryConnect.Args)
	if err := RunWithLogCapture(ctx, kopiaRepositoryConnect, nil); err != nil {
		return fmt.Errorf("while connecting to Kopia repository: %w", err)
	}

	return nil
}

func getCommonS3Args(cfg S3RepoOpts) ([]string, error) {
	doNotUseTLS := false
	shortenedEndpoint := ""

	if cfg.Endpoint != "" {
		endpointURL, err := url.Parse(cfg.Endpoint)
		if err != nil {
			return nil, fmt.Errorf("invalid endpoint URL %q: %w", endpointURL, err)
		}

		doNotUseTLS = strings.ToLower(endpointURL.Scheme) != "https"
		shortenedEndpoint = endpointURL.Host
	}

	args := make([]string, 0, 8)
	args = append(args,
		"--bucket="+cfg.BucketName,
		"--cache-directory="+cfg.CacheDirectory,
		"--prefix="+path.Join(cfg.Prefix, "base")+"/",
		"--disable-file-logging",
	)

	if cfg.Region != "" {
		args = append(args, "--region="+cfg.Region)
	}

	if shortenedEndpoint != "" {
		args = append(args, "--endpoint="+shortenedEndpoint)
	}
	if doNotUseTLS {
		args = append(args, "--disable-tls")
	}
	if cfg.CustomCABundleFile != "" {
		args = append(args, "--root-ca-pem-path="+cfg.CustomCABundleFile)
	}

	return args, nil
}

func getCommonS3Env(cfg S3RepoOpts) []string {
	return []string{
		"KOPIA_LOG_DIR=" + cfg.CacheDirectory,
		"KOPIA_PASSWORD=" + cfg.EncryptionPassword,
		"AWS_ACCESS_KEY_ID=" + cfg.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY=" + cfg.SecretAccessKey,
		"AWS_SESSION_TOKEN=" + cfg.SessionToken,
		"KOPIA_CHECK_FOR_UPDATES=false",
	}
}
