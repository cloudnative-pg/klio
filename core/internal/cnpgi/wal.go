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

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cnpg-i/pkg/wal"
	"github.com/cloudnative-pg/machinery/pkg/log"
)

type walServiceImplementation struct {
	wal.UnimplementedWALServer

	enableDebug bool
}

// GetCapabilities implements the WALService interface.
func (w walServiceImplementation) GetCapabilities(
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
func (w walServiceImplementation) Restore(
	ctx context.Context,
	request *wal.WALRestoreRequest,
) (*wal.WALRestoreResult, error) {
	contextLogger := log.FromContext(ctx).WithName("wal_restore")
	walName := request.GetSourceWalName()
	destinationPath := request.GetDestinationFileName()

	contextLogger.Info("WAL restore operation started", "walName", walName, "destinationPath", destinationPath)

	if walName == "" || destinationPath == "" {
		contextLogger.Warning("WAL restore operation failed. WAL name and destination file name must be specified")
		return nil, errors.New("source WAL name and destination file name must be provided")
	}

	args := []string{"get-wal", walName, destinationPath, "--partial=true"}
	if w.enableDebug {
		args = append(args, "--debug")
	}

	// We need to find out the WAL repository to use
	var cluster cnpgv1.Cluster
	if err := json.Unmarshal(request.GetClusterDefinition(), &cluster); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cluster definition: %w", err)
	}
	podName, ok := os.LookupEnv("PODNAME") // Ensure PODNAME is set in the environment
	if !ok {
		return nil, errors.New("PODNAME environment variable is not set")
	}
	confPath, err := getWalRepositoryConfigurationPath(&cluster, podName)
	if err != nil {
		return nil, fmt.Errorf("failed to get WAL repository: %w", err)
	}

	if confPath == "" {
		return nil, errors.New("no WAL repository found for the cluster")
	}

	contextLogger.Info("selected WAL repository configuration", "repositoryConfigPath", confPath)

	args = append(args, "--config", confPath)

	cmd := exec.CommandContext( //nolint: gosec
		ctx,
		"klio",
		args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to execute klio get-wal command: %w, stderr: %s", err, stderr.String())
	}

	return &wal.WALRestoreResult{}, nil
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
