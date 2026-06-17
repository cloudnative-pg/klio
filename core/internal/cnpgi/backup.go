package cnpgi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/cloudnative-pg/cnpg-i/pkg/backup"
	"github.com/cloudnative-pg/machinery/pkg/log"
	pgTime "github.com/cloudnative-pg/machinery/pkg/postgres/time"
	"github.com/cloudnative-pg/machinery/pkg/types"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/cloudnative-pg/klio/core/internal/backupfailure"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

// backupServiceImplementation is the implementation
// of the Backup CNPG capability.
type backupServiceImplementation struct {
	backup.UnimplementedBackupServer

	InstanceName string
	Tier2        bool
}

// GetCapabilities implements the Backup service interface.
func (b backupServiceImplementation) GetCapabilities(
	_ context.Context, _ *backup.BackupCapabilitiesRequest,
) (*backup.BackupCapabilitiesResult, error) {
	return &backup.BackupCapabilitiesResult{
		Capabilities: []*backup.BackupCapability{
			{
				Type: &backup.BackupCapability_Rpc{
					Rpc: &backup.BackupCapability_RPC{
						Type: backup.BackupCapability_RPC_TYPE_BACKUP,
					},
				},
			},
		},
	}, nil
}

// Backup implements the Backup interface.
func (b backupServiceImplementation) Backup(
	ctx context.Context,
	request *backup.BackupRequest,
) (*backup.BackupResult, error) {
	ctx, span := tracer.Start(ctx, opentelemetry.BackupSpan)
	defer span.End()

	// Step 1: get and apply the retention policies
	var cluster cnpgv1.Cluster
	if err := json.Unmarshal(request.GetClusterDefinition(), &cluster); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cluster definition: %w", err)
	}

	r, err := extractTier1RetentionFromConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to extract retention policy from configuration: %w", err)
	}

	if err = b.setRetentionPolicy(ctx, r); err != nil {
		// Yes this is intentional. If we don't set the retention policies from
		// the configuration file, it is not a major issue. We can continue with the backup.
		// The eventual error will be logged into the setRetentionPolicy function
		log.Error(err, "failed to set retention policy")
	}

	// Step 2: starting the backup
	backupName := fmt.Sprintf("backup-%v", pgTime.ToCompactISO8601(time.Now()))
	isPrimary := b.InstanceName == cluster.Status.CurrentPrimary

	role := "standby"
	if isPrimary {
		role = "primary"
	}
	log.FromContext(ctx).Info("Detected pod role for backup",
		"role", role,
		"podName", b.InstanceName,
		"currentPrimary", cluster.Status.CurrentPrimary,
	)

	span.SetAttributes(
		attribute.String("backup.name", backupName),
		attribute.String("backup.role", role),
	)

	backupStart := time.Now()
	recordBackupStart(ctx)
	defer recordBackupFinished(ctx)

	metadata, err := b.runBackup(
		ctx,
		backupName,
		isPrimary,
	)
	if err != nil {
		recordBackupFailure(ctx, time.Since(backupStart), err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "backup failed")

		return nil, err
	}

	// Step 3: verify the backup on tier1
	corruption, verifyErr := b.runVerify(ctx, backupName)
	if corruption {
		recordVerificationFailure(ctx)
		recordBackupFailure(ctx, time.Since(backupStart), verifyErr)
		span.RecordError(verifyErr)
		span.SetStatus(codes.Error, "verification detected corruption")

		return nil, verifyErr
	}

	recordVerificationSuccess(ctx)
	recordBackupSuccess(ctx, time.Since(backupStart))

	// Step 4: trigger maintenance
	b.triggerMaintenance(ctx)

	return &backup.BackupResult{
		BackupName:        backupName,
		StartedAt:         metadata.StartedAt,
		StoppedAt:         metadata.StoppedAt,
		BackupLabelFile:   []byte(metadata.BackupLabel),
		TablespaceMapFile: []byte(metadata.TablespaceMap),
		Metadata:          metadata.Annotations,
		BeginLsn:          string(types.Int64ToLSN(metadata.StartLSN)),
		EndLsn:            string(types.Int64ToLSN(metadata.EndLSN)),
		BackupId:          backupName,
		InstanceId:        b.InstanceName,
		BeginWal:          metadata.StartWAL,
		EndWal:            metadata.EndWAL,
		Online:            true,
	}, nil
}

// classifyRunBackupError maps an error from the `klio backup run`
// subprocess to a failure category. A category reported by the
// subprocess via its exit code takes precedence over the context error.
func classifyRunBackupError(ctx context.Context, err error) backupfailure.Category {
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		if category, ok := backupfailure.ByExitCode(exitErr.ExitCode()); ok {
			return category
		}
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		switch {
		case errors.Is(ctxErr, context.DeadlineExceeded):
			return backupfailure.Timeout
		case errors.Is(ctxErr, context.Canceled):
			return backupfailure.Canceled
		}
	}

	return backupfailure.Unknown
}

func (b backupServiceImplementation) runBackup(
	ctx context.Context,
	backupName string,
	isPrimary bool,
) (*klioclient.BackupMetadata, error) {
	ctx, span := tracer.Start(ctx, opentelemetry.BackupRunSpan)
	defer span.End()

	contextLogger := log.FromContext(ctx)

	waitForWals := "--wait-for-wals=true"
	if !isPrimary {
		waitForWals = "--wait-for-wals=false"
	}

	args := []string{
		"backup",
		"run",
		"--config",
		backupRepositoryConfigPath,
		waitForWals,
		"-n",
		backupName,
	}

	if b.Tier2 {
		args = append(args, "--enable-tier2-backup")
	}

	contextLogger.Info("Starting Klio backup", "backupName", backupName, "args", args)

	klioPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to determine klioPath path: %w", err)
	}
	//nolint:gosec
	cmd := exec.CommandContext(ctx, klioPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to execute klio backup run command: %w", err)
	}
	contextLogger.Info("Backup completed, getting metadata", "backupName", backupName)

	//nolint:gosec
	cmd = exec.CommandContext(
		ctx,
		klioPath,
		"backup",
		"get-metadata",
		"--config",
		backupRepositoryConfigPath,
		backupName)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	cmd.Env = filterOTelEnv(os.Environ())
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to execute klio backup get-metadata command: %w", err)
	}

	var metadata klioclient.BackupMetadata
	if err := json.Unmarshal(stdout.Bytes(), &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse backup metadata: %w", err)
	}

	return &metadata, nil
}

var (
	// ErrVerificationCorruption is returned when backup verification detects corruption.
	ErrVerificationCorruption = errors.New("backup verification detected corruption")

	// ErrPodNameNotSet is returned when the POD_NAME environment variable is not set.
	ErrPodNameNotSet = errors.New("POD_NAME environment variable is not set")
)

// verifyBackoff defines the retry strategy for backup verification.
//
//nolint:gochecknoglobals
var verifyBackoff = wait.Backoff{
	Duration: 5 * time.Second,
	Factor:   2,
	Steps:    3,
}

// runVerify runs backup verification on tier1 and returns whether corruption was
// detected. Infrastructure errors are retried with exponential backoff.
// On persistent infrastructure errors the backup continues (corruption=false)
// with a logged warning.
func (b backupServiceImplementation) runVerify(ctx context.Context, backupName string) (bool, error) {
	ctx, span := tracer.Start(ctx, opentelemetry.BackupVerifySpan)
	defer span.End()

	contextLogger := log.FromContext(ctx)

	klioPath, err := os.Executable()
	if err != nil {
		contextLogger.Error(err, "Failed to determine klio path, skipping verification")
		return false, nil
	}

	var lastInfraErr error

	retryErr := wait.ExponentialBackoffWithContext(ctx, verifyBackoff, func(ctx context.Context) (bool, error) {
		cmd := exec.CommandContext( //nolint:gosec
			ctx, klioPath, "backup", "verify", backupName, "--tiers=tier1", "--config", backupRepositoryConfigPath,
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if runErr := cmd.Run(); runErr != nil {
			exitErr, ok := errors.AsType[*exec.ExitError](runErr)
			if ok && exitErr.ExitCode() == backupfailure.Verification.ExitCode {
				return false, fmt.Errorf("%w: %w", ErrVerificationCorruption, runErr)
			}

			lastInfraErr = runErr
			contextLogger.Info("Backup verification encountered an infrastructure error, retrying",
				"error", runErr,
			)

			return false, nil
		}

		return true, nil
	})

	if retryErr != nil {
		if errors.Is(retryErr, ErrVerificationCorruption) {
			return true, retryErr
		}

		contextLogger.Info("Backup verification failed after retries, backup will continue",
			"error", lastInfraErr,
		)
	}

	return false, nil
}

func (b backupServiceImplementation) triggerMaintenance(ctx context.Context) {
	ctx, span := tracer.Start(ctx, opentelemetry.BackupMaintenanceSpan)
	defer span.End()

	contextLogger := log.FromContext(ctx)

	klioPath, err := os.Executable()
	if err != nil {
		contextLogger.Error(err, "failed to determine klio path, skipping maintenance")
		return
	}

	contextLogger.Info("Starting Klio backup maintenance")
	//nolint:gosec
	cmd := exec.CommandContext(ctx, klioPath, "backup", "maintenance", "--config", backupRepositoryConfigPath)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		contextLogger.Error(err, "failed to execute klio backup maintenance command, skipping")
	}
}

//nolint:cyclop
func (b backupServiceImplementation) setRetentionPolicy(ctx context.Context, r *Retention) error {
	contextLogger := log.FromContext(ctx)

	if r.IsEmpty() {
		contextLogger.Info("Skipping retention policy creation")
		return nil
	}

	klioPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to determine klio path: %w", err)
	}

	klioArgs := []string{
		"retention", "set", "--config", backupRepositoryConfigPath,
	}
	if r.KeepAnnual != nil {
		klioArgs = append(klioArgs, "--keep-annual", strconv.Itoa(*r.KeepAnnual))
	}
	if r.KeepDaily != nil {
		klioArgs = append(klioArgs, "--keep-daily", strconv.Itoa(*r.KeepDaily))
	}
	if r.KeepHourly != nil {
		klioArgs = append(klioArgs, "--keep-hourly", strconv.Itoa(*r.KeepHourly))
	}
	if r.KeepLatest != nil {
		klioArgs = append(klioArgs, "--keep-latest", strconv.Itoa(*r.KeepLatest))
	}
	if r.KeepWeekly != nil {
		klioArgs = append(klioArgs, "--keep-weekly", strconv.Itoa(*r.KeepWeekly))
	}
	if r.KeepMonthly != nil {
		klioArgs = append(klioArgs, "--keep-monthly", strconv.Itoa(*r.KeepMonthly))
	}

	contextLogger.Info("Executing klio retention set", "args", klioArgs)
	//nolint:gosec
	cmd := exec.CommandContext(ctx, klioPath, klioArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to execute 'klio retention set' command: %w", err)
	}

	contextLogger.Info("Executing klio retention get")
	//nolint:gosec
	cmd = exec.CommandContext(ctx, klioPath, "retention", "get", "--config", backupRepositoryConfigPath)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to execute 'klio retention get' command: %w", err)
	}

	contextLogger.Info("Effective retention policy", "effectivePolicy", stdout.String())

	return nil
}

// filterOTelEnv returns a copy of env with all OTEL_ variables removed.
// This prevents subprocesses from inheriting OpenTelemetry configuration
// (e.g. OTEL_METRICS_EXPORTER=console) that could write to stdout and
// corrupt captured command output.
func filterOTelEnv(env []string) []string {
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, "OTEL_") {
			filtered = append(filtered, e)
		}
	}

	return filtered
}
