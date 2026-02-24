package cnpgi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cnpg-i/pkg/wal"
	"github.com/cloudnative-pg/machinery/pkg/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/grpcclient"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

var errWALNotFound = errors.New("wal not found")

type walServiceImplementation struct {
	wal.UnimplementedWALServer

	mu sync.Mutex

	opts WALCapabilityOptions

	availableTiers   []string
	currentTierIndex int

	mgr *grpcClientManager
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
	err := w.mgr.restoreWAL(ctx, restoreWALOptions{
		configFile: configPath,
		tier:       w.getCurrentTier(),
	}, walName, destinationPath)

	// Only try the alternate tier if the WAL was not found and we have multiple tiers
	if !errors.Is(err, errWALNotFound) || len(w.availableTiers) == 1 {
		return err
	}

	// Let's try the other tier
	w.skipCurrentTier()
	err = w.mgr.restoreWAL(ctx, restoreWALOptions{
		configFile: configPath,
		tier:       w.getCurrentTier(),
	}, walName, destinationPath)

	return err
}

type restoreWALOptions struct {
	configFile string
	tier       string
}

type grpcClientManager struct {
	m           sync.Mutex
	connections map[restoreWALOptions]*grpcclient.Connection
}

func newGRPCClientManager() *grpcClientManager {
	return &grpcClientManager{
		connections: make(map[restoreWALOptions]*grpcclient.Connection),
	}
}

func (mgr *grpcClientManager) getConnection(opts restoreWALOptions) (*grpcclient.Connection, error) {
	mgr.m.Lock()
	defer mgr.m.Unlock()

	if c, ok := mgr.connections[opts]; ok {
		return c, nil
	}

	configFile, err := os.Open(opts.configFile)
	if err != nil {
		return nil, fmt.Errorf("while loading config file %q: %w", opts.configFile, err)
	}
	defer func() {
		_ = configFile.Close()
	}()

	configuration, err := config.DecodeYAML(configFile)
	if err != nil {
		return nil, fmt.Errorf("while decoding config file %q: %w", opts.configFile, err)
	}

	var address string
	switch opts.tier {
	case "tier1":
		address = configuration.Client.Wal.Address

	case "tier2":
		address = configuration.Client.Wal.Tier2Address

	default:
		return nil, fmt.Errorf("unknown tier %q", opts.tier)
	}

	client, err := grpcclient.Connect(&configuration.Client, address)
	if err != nil {
		return nil, fmt.Errorf("while connecting to the Klio server: %w", err)
	}

	mgr.connections[opts] = client

	return client, nil
}

func (mgr *grpcClientManager) restoreWAL(
	ctx context.Context,
	opts restoreWALOptions,
	walName string,
	targetFileName string,
) error {
	contextLogger := log.FromContext(ctx).
		WithValues(
			"configFile", opts.configFile,
			"tier", opts.tier,
			"walName", walName,
			"targetFileName", targetFileName,
		)

	contextLogger.Debug("Restoring WAL file")
	start := time.Now()
	err := mgr.internalRestoreWAL(ctx, opts, walName, targetFileName)
	duration := time.Since(start)

	if err != nil {
		contextLogger.Info("Error while restoring WAL File", "duration", duration, "err", err)
	} else {
		contextLogger.Info("Restored WAL File", "duration", duration)
	}

	return err
}

func (mgr *grpcClientManager) internalRestoreWAL(
	ctx context.Context,
	opts restoreWALOptions,
	walName string,
	targetFileName string,
) error {
	contextLogger := log.FromContext(ctx)

	client, err := mgr.getConnection(opts)
	if err != nil {
		return err
	}

	output, err := os.OpenFile(targetFileName, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec
	if err != nil {
		return fmt.Errorf("cannot open file %s: %w", targetFileName, err)
	}
	defer func() {
		if closeErr := output.Close(); closeErr != nil {
			contextLogger.Error(closeErr, "While closing WAL file")
		}
	}()

	// Try to download the requested WAL file. If we did it, everything
	// is fine.
	err = client.GetWALStreaming(ctx, walName, output)
	switch {
	case errors.Is(err, klioclient.ErrMissingWALFile):
		// Let's try downloading the partial file

	case err != nil:
		return fmt.Errorf("unknown error: %w", err)

	default:
		return nil
	}

	// Let's try downloading the partial file
	walName += ".partial"
	err = client.GetWALStreaming(ctx, walName, output)

	var incompleteError klioclient.IncompleteTransmissionError
	switch {
	case errors.As(err, &incompleteError):
		return err

	case errors.Is(err, klioclient.ErrMissingWALFile):
		return errWALNotFound

	case err != nil:
		return fmt.Errorf("while downloading WAL %q: %w", walName, err)
	}

	return nil
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
