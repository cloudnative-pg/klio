package cnpgi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cnpg-i/pkg/wal"
	"github.com/cloudnative-pg/machinery/pkg/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var errWALNotFound = errors.New("wal not found")

type walServiceImplementation struct {
	wal.UnimplementedWALServer

	mu sync.Mutex

	opts WALCapabilityOptions

	availableTiers   []string
	currentTierIndex int
}

// GetCapabilities implements the WALService interface.
func (w *walServiceImplementation) GetCapabilities(
	_ context.Context,
	_ *wal.WALCapabilitiesRequest,
) (*wal.WALCapabilitiesResult, error) {
	return &wal.WALCapabilitiesResult{
		Capabilities: []*wal.WALCapability{
			{
				Type: &wal.WALCapability_Rpc{
					Rpc: &wal.WALCapability_RPC{
						Type: wal.WALCapability_RPC_TYPE_RESTORE_WAL,
					},
				},
			},
		},
	}, nil
}

// Restore implements the WALService interface.
func (w *walServiceImplementation) Restore(
	ctx context.Context,
	request *wal.WALRestoreRequest,
) (*wal.WALRestoreResult, error) {
	contextLogger := log.FromContext(ctx).WithName("wal_restore")
	walName := request.GetSourceWalName()
	destinationPath := request.GetDestinationFileName()

	if walName == "" || destinationPath == "" {
		contextLogger.Warning("WAL restore operation failed. WAL name and destination file name must be specified")
		return nil, errors.New("source WAL name and destination file name must be provided")
	}

	// We need to find out the WAL repository to use
	var cluster cnpgv1.Cluster
	if err := json.Unmarshal(request.GetClusterDefinition(), &cluster); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cluster definition: %w", err)
	}
	podName, ok := os.LookupEnv("POD_NAME") // Ensure PODNAME is set in the environment
	if !ok {
		return nil, errors.New("POD_NAME environment variable is not set")
	}
	confPath, err := getWalRepositoryConfigurationPath(&cluster, podName)
	if err != nil {
		return nil, fmt.Errorf("failed to get WAL repository: %w", err)
	}

	if confPath == "" {
		return nil, errors.New("no WAL repository found for the cluster")
	}

	err = w.restoreWAL(ctx, walName, destinationPath, confPath)
	if errors.Is(err, errWALNotFound) {
		return &wal.WALRestoreResult{}, status.Errorf(codes.NotFound, "WAL file not found: %q", walName)
	}
	if err != nil {
		return nil, err
	}

	return &wal.WALRestoreResult{}, nil
}

func (w *walServiceImplementation) getCurrentTier() string {
	return w.availableTiers[w.currentTierIndex]
}

func (w *walServiceImplementation) skipCurrentTier() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.currentTierIndex = (w.currentTierIndex + 1) % len(w.availableTiers)
}

func (w *walServiceImplementation) restoreWAL(
	ctx context.Context,
	walName, destinationPath string,
	configPath string,
) error {
	err := internalRestoreWAL(ctx, restoreWALOptions{
		walName:         walName,
		destinationPath: destinationPath,
		configFile:      configPath,
		debug:           w.opts.Debug,
		tier:            w.getCurrentTier(),
	})

	// Only try the alternate tier if the WAL was not found and we have multiple tiers
	if !errors.Is(err, errWALNotFound) || len(w.availableTiers) == 1 {
		return err
	}

	// Let's try the other tier
	w.skipCurrentTier()
	err = internalRestoreWAL(ctx, restoreWALOptions{
		walName:         walName,
		destinationPath: destinationPath,
		configFile:      configPath,
		debug:           w.opts.Debug,
		tier:            w.getCurrentTier(),
	})

	return err
}

type restoreWALOptions struct {
	walName         string
	destinationPath string
	configFile      string

	debug bool
	tier  string
}

func internalRestoreWAL(ctx context.Context, opts restoreWALOptions) error {
	args := []string{
		"get-wal",
		opts.walName,
		opts.destinationPath,
		"--partial=true",
		"--tier=" + opts.tier,
	}
	if opts.debug {
		args = append(args, "--debug")
	}

	// We need to find out the WAL repository to use
	args = append(args, "--config", opts.configFile)

	cmd := exec.CommandContext( //nolint: gosec
		ctx,
		"klio",
		args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	contextLogger := log.FromContext(ctx).WithName("wal_restore")
	contextLogger.Info("Starting get-wal", "args", args, "opts", opts)

	var exitError *exec.ExitError
	err := cmd.Run()
	switch {
	case errors.As(err, &exitError) && exitError.ExitCode() == 4:
		return errWALNotFound

	case err != nil:
		return fmt.Errorf(
			"failed to execute klio get-wal command: %w, stdout: %q, stderr: %q",
			err,
			stdout.String(),
			stderr.String(),
		)

	default:
		return nil
	}
}

func getWalRepositoryConfigurationPath(cluster *cnpgv1.Cluster, instanceName string) (string, error) {
	var promotionToken string
	if cluster.Spec.ReplicaCluster != nil {
		promotionToken = cluster.Spec.ReplicaCluster.PromotionToken
	}

	var repositoryName string
	var err error
	switch {
	case promotionToken != "" && cluster.Status.LastPromotionToken != promotionToken:
		// This is a replica cluster that is being promoted to a primary cluster
		// Recover from the replica source
		repositoryName, err = getSourceRepositoryConfigPath(cluster)

	case cluster.IsReplica() && cluster.Status.CurrentPrimary == instanceName:
		// Designated primary on replica cluster
		repositoryName, err = getSourceRepositoryConfigPath(cluster)

	// If we have no primary, we assume this is a bootstrap
	case cluster.Status.CurrentPrimary == "":
		repositoryName, err = getBootstrapRepositoryConfigPath(cluster)

	default:
		// Using cluster default
		repositoryName = backupRepositoryConfigPath
	}

	return repositoryName, err
}

const backupRepositoryConfigPath = "/var/lib/postgresql/klio/klio-archive"

func getSourceRepositoryConfigPath(cluster *cnpgv1.Cluster) (string, error) {
	if cluster.Spec.ReplicaCluster == nil {
		return "", fmt.Errorf("cluster %s is not a replica cluster", cluster.Name)
	}
	source := cluster.Spec.ReplicaCluster.Source

	return filepath.Clean(filepath.Join("/var/lib/postgresql/klio", source)), nil
}

func getBootstrapRepositoryConfigPath(cluster *cnpgv1.Cluster) (string, error) {
	if cluster.Spec.Bootstrap == nil {
		return "", fmt.Errorf("cluster %s does not have a bootstrap configured", cluster.Name)
	}
	if cluster.Spec.Bootstrap.Recovery == nil {
		return "", fmt.Errorf("cluster %s does not have a bootstrap recovery configured", cluster.Name)
	}
	if cluster.Spec.Bootstrap.Recovery.Source == "" {
		return "", fmt.Errorf("cluster %s does not have a bootstrap recovery source configured", cluster.Name)
	}

	return filepath.Clean(filepath.Join("/var/lib/postgresql/klio", cluster.Spec.Bootstrap.Recovery.Source)), nil
}
