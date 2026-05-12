package cnpgi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cnpg-i/pkg/wal"
	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spf13/afero"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/grpcclient"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

var errWALNotFound = errors.New("wal not found")

type tier string

const (
	tier1 tier = "tier1"
	tier2 tier = "tier2"
)

type walServiceImplementation struct {
	wal.UnimplementedWALServer

	opts        WALCapabilityOptions
	currentTier atomic.Value

	mgr *grpcClientManager
}

func newWalServiceImplementation(mgr *grpcClientManager, opts WALCapabilityOptions) walServiceImplementation {
	result := walServiceImplementation{
		opts: opts,
		mgr:  mgr,
	}
	result.currentTier.Store(tier1)

	return result
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
	startCall := time.Now()
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

	contextLogger.Info("WAL.Restore", "walName", request.GetSourceWalName(), "duration", time.Since(startCall))

	return &wal.WALRestoreResult{}, nil
}

func (w *walServiceImplementation) restoreWAL(
	ctx context.Context,
	walName, destinationPath string,
	configPath string,
) error {
	cfg, err := config.NewFromFile(afero.NewOsFs(), configPath)
	if err != nil {
		return fmt.Errorf("while loading configuration from file %q: %w", configPath, err)
	}

	tiers := availableTiers(cfg)
	if len(tiers) == 0 {
		return errors.New("no WAL tier configured")
	}

	// Try the previously-successful tier first, when both are available.
	currentTier := w.currentTier.Load()
	if len(tiers) > 1 && tiers[0] != currentTier {
		tiers[0], tiers[1] = tiers[1], tiers[0]
	}

	for _, t := range tiers {
		err := w.mgr.restoreWAL(ctx, walRestoreOptions{
			configFile: configPath,
			targetTier: t,
		}, walName, destinationPath)
		if err == nil {
			w.currentTier.Store(t)
			return nil
		}
		if !errors.Is(err, errWALNotFound) {
			return err
		}
	}

	return errWALNotFound
}

// availableTiers returns the tiers the user has opted in to as recovery
// sources. The gating is asymmetric on purpose: the operator only sets
// Wal.Address when the user wants to archive to tier1 (which implies
// restoring from it), so address presence is sufficient there. Wal.Tier2Address
// is set whenever the user enables tier2 for backup OR recovery, so we must
// additionally consult Tier2RecoveryEnabled to avoid pulling restores from
// a backup-only tier2.
func availableTiers(cfg *config.Data) []tier {
	var tiers []tier
	if cfg.Client.Wal.Address != "" {
		tiers = append(tiers, tier1)
	}
	if cfg.Tier2RecoveryEnabled && cfg.Client.Wal.Tier2Address != "" {
		tiers = append(tiers, tier2)
	}

	return tiers
}

type walRestoreOptions struct {
	configFile string
	targetTier tier
}

// walRestoreClient holds the connection and prefetcher for WAL restoration.
type walRestoreClient struct {
	connection *grpcclient.Connection
	prefetcher *walPrefetcher
}

type grpcClientManager struct {
	m       sync.Mutex
	clients map[walRestoreOptions]*walRestoreClient
}

func newGRPCClientManager() *grpcClientManager {
	return &grpcClientManager{
		clients: make(map[walRestoreOptions]*walRestoreClient),
	}
}

func (mgr *grpcClientManager) getClient(ctx context.Context, opts walRestoreOptions) (*walRestoreClient, error) {
	mgr.m.Lock()
	defer mgr.m.Unlock()

	if c, ok := mgr.clients[opts]; ok {
		return c, nil
	}

	contextLogger := log.FromContext(ctx)

	configuration, err := config.NewFromFile(afero.NewOsFs(), opts.configFile)
	if err != nil {
		return nil, fmt.Errorf("while loading configuration from file %q: %w", opts.configFile, err)
	}

	var address string
	switch opts.targetTier {
	case tier1:
		address = configuration.Client.Wal.Address
	case tier2:
		address = configuration.Client.Wal.Tier2Address
	default:
		return nil, fmt.Errorf("unknown tier %q", opts.targetTier)
	}
	if address == "" {
		return nil, fmt.Errorf("missing address for tier %q", opts.targetTier)
	}

	connection, err := grpcclient.Connect(&configuration.Client, address)
	if err != nil {
		return nil, fmt.Errorf("while connecting to the Klio server: %w", err)
	}

	// Set up spool directory for this configuration.
	spoolDir, err := mgr.setupSpoolDir(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("while setting up spool directory: %w", err)
	}

	// Read prefetch settings from the loaded configuration.
	prefetchCount := configuration.WALPrefetch.Count
	maxConcurrentDownloads := configuration.WALPrefetch.MaxConcurrentDownloads

	client := &walRestoreClient{
		connection: connection,
		//nolint:contextcheck // prefetcher creates its own internal context for the download pool
		prefetcher: newWALPrefetcher(spoolDir, connection, prefetchCount, maxConcurrentDownloads),
	}
	mgr.clients[opts] = client

	contextLogger.Info("Created WAL restore client",
		"targetTier", opts.targetTier,
		"configFile", opts.configFile,
		"spoolDir", spoolDir,
		"prefetchCount", prefetchCount,
		"maxConcurrentDownloads", maxConcurrentDownloads,
	)

	return client, nil
}

// setupSpoolDir creates and cleans the spool directory for a configuration.
func (mgr *grpcClientManager) setupSpoolDir(ctx context.Context, opts walRestoreOptions) (string, error) {
	pgdata := os.Getenv("PGDATA")
	if pgdata == "" {
		return "", errors.New("PGDATA environment variable is not set")
	}

	spoolBase := filepath.Join(pgdata, "pg_wal", spoolDirName)
	spoolDir := filepath.Join(spoolBase, spoolSubdir(opts))

	// Create the spool directory if it doesn't exist.
	if err := os.MkdirAll(spoolDir, 0o700); err != nil { //nolint:gosec
		return "", fmt.Errorf("creating spool directory %q: %w", spoolDir, err)
	}

	// Clean up any leftover files from previous runs.
	if err := cleanupSpoolDir(spoolDir); err != nil {
		log.FromContext(ctx).Error(err, "Failed to clean up spool directory", "spoolDir", spoolDir)
		// Continue anyway - leftover files shouldn't prevent operation.
	}

	return spoolDir, nil
}

func (mgr *grpcClientManager) restoreWAL(
	ctx context.Context,
	opts walRestoreOptions,
	walName string,
	targetFileName string,
) error {
	client, err := mgr.getClient(ctx, opts)
	if err != nil {
		return err
	}

	return client.prefetcher.Request(ctx, walName, targetFileName)
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
