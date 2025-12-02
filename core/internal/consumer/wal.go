package consumer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/queue"
	"github.com/cloudnative-pg/klio/core/internal/repository"
)

// WAL represents a WAL consumer.
type WAL struct {
	metrics *repository.Metrics
	opts    *WALOptions
}

// WALOptions are the configuration of the WAL consumer.
type WALOptions struct {
	// The queue to be used
	Queue *queue.Conn

	// A connection to tier 1
	Tier1 *repository.Connection

	// A connection to tier 2
	Tier2 *repository.Connection
}

// NewWAL creates a new WAL consumer.
func NewWAL(opts *WALOptions) *WAL {
	return &WAL{
		metrics: NewMetrics(),
		opts:    opts,
	}
}

// Run starts the consumer until the context is canceled or the
// SIGINT signal arrives.
func (d *WAL) Run(ctx context.Context) error {
	consumerCtx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	return d.opts.Queue.ConsumeWALReceivedMessages(consumerCtx, d.walHandler)
}

func (d *WAL) walHandler(ctx context.Context, task *queue.WALTask) error {
	logger := log.FromContext(ctx).WithValues("task", task)
	logger.Info("Archiving WAL file")

	reader, err := repository.NewReader(d.opts.Tier1,
		task.ClusterName,
		task.WALName,
		tracer,
	)
	if err != nil {
		return fmt.Errorf("while creating a new WAL reader: %w", err)
	}
	defer func() {
		if err := reader.Close(); err != nil {
			logger.Info("Error while closing the WAL file from Tier 1")
		}
	}()

	writer, err := d.opts.Tier2.NewWriter(task.ClusterName,
		task.WALName,
		reader.GetFileLength(),
		d.metrics,
		tracer,
	)
	if err != nil {
		return fmt.Errorf("while creating a new WAL writer for Tier 2: %w", err)
	}
	defer func() {
		if err := writer.CloseMarkDone(); err != nil {
			logger.Error(err, "Error while closing the WAL file from Tier 2")
		}
	}()

	for {
		block, readError := reader.ReadBlock(ctx)
		if readError != nil && !errors.Is(readError, io.EOF) {
			return fmt.Errorf("error while reading WAL: %v", readError.Error())
		}

		writeError := writer.WriteBlock(ctx, block)
		if writeError != nil {
			return fmt.Errorf("error while writing WAL block: %v", writeError.Error())
		}

		if errors.Is(readError, io.EOF) {
			break
		}
	}

	return nil
}
