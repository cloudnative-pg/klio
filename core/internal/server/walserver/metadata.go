package walserver

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"google.golang.org/protobuf/proto"

	"github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/repository"
)

// ErrIncoherentMetadata happens when the cluster metadata is incoherent
// or not unmarshallable.
var ErrIncoherentMetadata = errors.New("incoherent cluster metadata")

// getClusterMetadata gets the cluster metadata for the cluster with the passed name.
func (w *Implementation) getClusterMetadata(ctx context.Context, clusterName string) (*grpc.ClusterMetadata, error) {
	logger := log.FromContext(ctx)

	walReader, err := repository.NewReader(w.conn, clusterName, metadataFileName, tracer)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := walReader.Close(); err != nil {
			logger.Error(err, "Error while closing metadata file for read", "clusterName", clusterName)
		}
	}()

	data, err := walReader.ReadBlock(ctx)
	if err != nil {
		return nil, err
	}

	var metadata grpc.ClusterMetadata
	if err := proto.Unmarshal(data, &metadata); err != nil {
		return nil, ErrIncoherentMetadata
	}

	return &metadata, nil
}

// writeClusterMetadata gets the cluster metadata for the cluster with the passed name.
func (w *Implementation) writeClusterMetadata(
	ctx context.Context,
	clusterName string,
	metadata *grpc.ClusterMetadata,
) error {
	logger := log.FromContext(ctx)
	data, err := proto.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("internal error while marshalling protobuf data: %w", err)
	}

	walWriter, err := w.conn.NewWriter(clusterName, metadataFileName, uint64(len(data)), w.metrics, tracer)
	if err != nil {
		return err
	}

	defer func() {
		if err := walWriter.CloseMarkDone(); err != nil {
			logger.Error(err, "Error while closing metadata file for read", "clusterName", clusterName)
		}
	}()

	if err := walWriter.WriteBlock(ctx, data); err != nil {
		return err
	}

	return nil
}
