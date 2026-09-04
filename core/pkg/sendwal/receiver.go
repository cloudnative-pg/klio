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

package sendwal

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/postgres"
	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/cloudnative-pg/machinery/pkg/types"
	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/cloudnative-pg/klio/core/pkg/sendwal/buffer"
	"github.com/cloudnative-pg/klio/core/pkg/sendwal/infrastructure"
)

// downloadHistoryFileSpanName identifies the span emitted while fetching and
// storing timeline history files.
const downloadHistoryFileSpanName = "sendwal.downloadHistoryFiles"

// ReplicationCoordinator negotiates the WAL stream with whatever system
// ultimately owns the received WAL data. Implementations are expected to
// talk to that destination over whichever transport it exposes (a gRPC
// service, an HTTP API, a local filesystem, ...); this package only needs
// the three operations below.
type ReplicationCoordinator interface {
	// RequestStart negotiates the replication start position with the
	// destination, given the WAL file name the source is currently at. It
	// returns the WAL file name the destination wants the stream to
	// (re)start from.
	RequestStart(ctx context.Context, clusterName, systemID, currentWALName string) (string, error)

	// ResetStream tells the destination to reset its replication status,
	// given the WAL file name the source is currently at. It returns the
	// WAL file name known to the destination, for logging purposes.
	ResetStream(ctx context.Context, clusterName, systemID, currentWALName string) (string, error)

	// StoreHistoryFile stores a timeline history file at the destination.
	StoreHistoryFile(ctx context.Context, name string, content []byte) error
}

// HandlerFactory creates the buffer.Handler that will receive WAL data once
// replication starts (or restarts, after a timeline switch) for the given
// timeline and WAL segment size.
type HandlerFactory func(tli int, segmentSize uint64) buffer.Handler

// Options carries the receiver's tunables. Callers own filling this in from
// whatever configuration mechanism they use.
type Options struct {
	// Slot is the name of the physical replication slot to use. It is
	// created if it does not already exist.
	Slot string

	// ClusterName identifies the PostgreSQL cluster to the
	// ReplicationCoordinator.
	ClusterName string

	// BufferSize is the maximum size, in bytes, of the in-memory WAL
	// buffer before it is automatically flushed.
	BufferSize int

	// FlushTimeout is the interval after which buffered WAL data is
	// automatically flushed, even if BufferSize has not been reached.
	FlushTimeout time.Duration

	// StandbyMessageTimeout is the interval after which a standby status
	// update is sent to the source, absent other activity that would
	// trigger one anyway.
	StandbyMessageTimeout time.Duration
}

// Process implements the WAL receiver service.
type Process struct {
	infrastructure *infrastructure.Postgres
	coordinator    ReplicationCoordinator
	newHandler     HandlerFactory
	options        Options
}

// New creates a new receiver. dsn is used to connect to the source
// PostgreSQL instance; coordinator negotiates streaming with the
// destination; newHandler builds the sink that will receive WAL bytes.
func New(
	dsn string,
	logger log.Logger,
	coordinator ReplicationCoordinator,
	newHandler HandlerFactory,
	options Options,
) *Process {
	return &Process{
		infrastructure: infrastructure.NewPostgres(dsn, logger),
		coordinator:    coordinator,
		newHandler:     newHandler,
		options:        options,
	}
}

// ResetReplicationStatus resets the replication status on the destination
// side and then drops the replication slot.
func (s *Process) ResetReplicationStatus(
	ctx context.Context,
) error {
	contextLogger := log.FromContext(ctx)

	conn, err := s.infrastructure.NewConn(ctx)
	if err != nil {
		return fmt.Errorf("while parsing DSN: %w", err)
	}
	defer func() {
		if closeErr := conn.Close(ctx); closeErr != nil {
			contextLogger.Error(closeErr, "error while closing the connection")
		}
	}()

	identifyData, err := pglogrepl.IdentifySystem(ctx, conn)
	if err != nil {
		return fmt.Errorf("while invoking IDENTIFY_SYSTEM: %w", err)
	}

	walSegmentSize, err := s.infrastructure.GetWalSegmentSize(ctx)
	if err != nil {
		return fmt.Errorf("while setting up replication: %w", err)
	}

	clientWALFileName, err := types.Int64ToLSN(uint64(identifyData.XLogPos)).WALFileName(
		int(identifyData.Timeline), walSegmentSize)
	if err != nil {
		return fmt.Errorf("while converting LSN to WAL file name: %q %w", identifyData.XLogPos, err)
	}

	result, err := s.coordinator.ResetStream(ctx, s.options.ClusterName, identifyData.SystemID, clientWALFileName)
	if err != nil {
		return fmt.Errorf("while invoking destination-side replication reset: %w", err)
	}

	contextLogger.Info(
		"Reset destination-side replication status",
		"walName", result)

	slotName := s.options.Slot
	if err := pglogrepl.DropReplicationSlot(
		ctx,
		conn,
		slotName,
		pglogrepl.DropReplicationSlotOptions{},
	); err != nil {
		return fmt.Errorf("while dropping replication slot: %w", err)
	}

	contextLogger.Info(
		"Dropped replication slot",
		"name", slotName)

	return nil
}

// Start the WAL receiver.
func (s *Process) Start(ctx context.Context) error {
	contextLogger := log.FromContext(ctx)

	conn, err := s.infrastructure.NewConn(ctx)
	if err != nil {
		return fmt.Errorf("while parsing DSN: %w", err)
	}
	defer func() {
		if closeErr := conn.Close(ctx); closeErr != nil {
			contextLogger.Error(closeErr, "Error while closing the connection")
		}
	}()

	walSegmentSize, err := s.infrastructure.GetWalSegmentSize(ctx)
	if err != nil {
		return fmt.Errorf("while setting up replication: %w", err)
	}

	identifyData, err := pglogrepl.IdentifySystem(ctx, conn)
	if err != nil {
		return fmt.Errorf("while executing identify_system: %w", err)
	}

	contextLogger.Info(
		"Current system identification data",
		"xlogFlushPosition", identifyData.XLogPos,
		"timeline", identifyData.Timeline,
		"systemID", identifyData.SystemID,
	)

	// Negotiate the starting point with the destination
	point, err := s.getReplicationStartPoint(ctx, conn, identifyData, walSegmentSize)
	if err != nil {
		return err
	}

	// We cannot guarantee to have all the history files available, so we ignore the error.
	// The wal receiver could have been configured later in the cluster lifecycle
	if histErr := s.downloadHistoryFiles(
		ctx,
		conn,
		max(identifyData.Timeline, point.timeline),
	); histErr != nil {
		contextLogger.Debug("Some timeline history files could not be processed", "innerErr", histErr.Error())
	}

	if err := s.ensureReplicationSlotExists(ctx, conn); err != nil {
		return err
	}

	return s.startReplication(ctx, conn, point, walSegmentSize)
}

func (s *Process) getReplicationStartPointFromDestination(
	ctx context.Context,
	conn *pgconn.PgConn,
	xlogFlushPos pglogrepl.LSN,
	segmentSize uint64,
) (pglogrepl.LSN, error) {
	contextLogger := log.FromContext(ctx)

	// Find the latest replication point reading the replication slot
	slotResult, err := ReadReplicationSlot(ctx, conn, s.options.Slot)
	if err != nil {
		return 0, fmt.Errorf("while reading replication slot: %w", err)
	}
	if slotResult.RestartLSN != 0 {
		startPoint := pglogrepl.LSN(uint64(slotResult.RestartLSN) & ^(segmentSize - 1))
		contextLogger.Debug(
			"Read replication slot",
			"lsn", startPoint)

		return slotResult.RestartLSN, nil
	}

	// If neither the destination nor the replication slot know a start
	// point, we use the XLOG flush position, taking care of starting
	// streaming from the beginning of the WAL file.
	//
	// This usually happens when we are running against this
	// PostgreSQL instance for the first time.
	contextLogger.Debug(
		"Current flush LSN",
		"xlogFlushPos", xlogFlushPos,
		"segmentSize", segmentSize,
	)

	return getStartWALLSN(xlogFlushPos, segmentSize), nil
}

type walCoordinate struct {
	timeline int32
	lsn      pglogrepl.LSN
}

func (s *Process) getReplicationStartPoint(
	ctx context.Context,
	conn *pgconn.PgConn,
	data pglogrepl.IdentifySystemResult,
	segmentSize uint64,
) (*walCoordinate, error) {
	contextLogger := log.FromContext(ctx)

	startLSN, err := s.getReplicationStartPointFromDestination(ctx, conn, data.XLogPos, segmentSize)
	if err != nil {
		return nil, err
	}

	currentWALFileName, err := types.Int64ToLSN(uint64(startLSN)).WALFileName(int(data.Timeline), segmentSize)
	if err != nil {
		return nil, fmt.Errorf("while converting LSN to WAL file name: %q %w", startLSN, err)
	}

	contextLogger.Debug("Requesting destination-side replication start",
		"cluster", s.options.ClusterName,
		"systemId", data.SystemID,
		"currentWAL", currentWALFileName)

	destinationWALFileName, err := s.coordinator.RequestStart(
		ctx, s.options.ClusterName, data.SystemID, currentWALFileName)
	if err != nil {
		return nil, fmt.Errorf("during destination-side replication point validation: %w", err)
	}

	contextLogger.Debug("Received destination-side replication start WAL", "name", destinationWALFileName)

	// Extract the timeline from the WAL file name. We remove the extension, as we may work on .partial files.
	// TODO: this should probably go to machinery
	segment, err := postgres.SegmentFromName(
		strings.TrimSuffix(destinationWALFileName, path.Ext(destinationWALFileName)))
	if err != nil {
		return nil, fmt.Errorf("while extracting segment from WAL file name %s: %w",
			destinationWALFileName,
			err)
	}
	tli := segment.Tli

	lsn, err := getReplicationStartFromWALFileName(destinationWALFileName, segmentSize)
	if err != nil {
		return nil, err
	}

	contextLogger.Info(
		"Negotiated replication start with the destination",
		"timeline", tli,
		"lsn", lsn,
	)

	return &walCoordinate{timeline: tli, lsn: lsn}, nil
}

// getStartWALLSN gets the LSN position of the start of the WAL file
// that contains the passed LSN.
// This is used to get the point where to start reading WALs given
// the current flush position.
func getStartWALLSN(xlogFlushPos pglogrepl.LSN, segmentSize uint64) pglogrepl.LSN {
	return pglogrepl.LSN(uint64(xlogFlushPos) & ^(segmentSize - 1))
}

func (s *Process) ensureReplicationSlotExists(
	ctx context.Context,
	conn *pgconn.PgConn,
) error {
	contextLogger := log.FromContext(ctx)

	slotResult, err := ReadReplicationSlot(ctx, conn, s.options.Slot)
	if err != nil {
		return fmt.Errorf("while reading replication slot: %w", err)
	}

	if len(slotResult.SlotType) > 0 {
		// we know the replication slot type, so this replication slot
		// really exists
		return nil
	}

	replicationSlotResult, err := pglogrepl.CreateReplicationSlot(
		ctx,
		conn,
		s.options.Slot,
		"", // output plugin name: this is meaningful only for logical replication
		pglogrepl.CreateReplicationSlotOptions{
			Temporary: false,
			Mode:      pglogrepl.PhysicalReplication,
		},
	)
	if err != nil {
		return fmt.Errorf("while creating temporary replication slot: %w", err)
	}

	contextLogger.Info(
		"Created replication slot",
		"consistentPoint", replicationSlotResult.ConsistentPoint,
		"name", replicationSlotResult.SlotName)

	return nil
}

func getReplicationStartFromWALFileName(walFileName string, segmentSize uint64) (pglogrepl.LSN, error) {
	walFileName, _ = strings.CutSuffix(walFileName, ".partial")

	fileName, err := types.LSNStartFromWALName(walFileName, segmentSize)
	if err != nil {
		return 0, fmt.Errorf("while parsing WAL file name %s: %w", walFileName, err)
	}

	lsn, err := fileName.Parse()
	if err != nil {
		return 0, fmt.Errorf("while parsing WAL file name %s: %w", walFileName, err)
	}

	return pglogrepl.LSN(lsn), nil
}

func (s *Process) downloadHistoryFiles(
	ctx context.Context,
	conn *pgconn.PgConn,
	currentTli int32,
) error {
	var errorList error

	ctx, span := tracer.Start(ctx, downloadHistoryFileSpanName,
		trace.WithAttributes(attribute.Int("currentTLI", int(currentTli))))
	defer span.End()

	contextLogger := log.FromContext(ctx)
	for tli := currentTli; tli > 1; tli-- {
		result, err := pglogrepl.TimelineHistory(ctx, conn, tli)
		if err != nil {
			span.RecordError(err)
			contextLogger.Error(err, "timeline history fetching failed, skipping", "tli", tli)
			errorList = errors.Join(errorList, err)

			continue
		}

		if err := s.coordinator.StoreHistoryFile(ctx, result.FileName, result.Content); err != nil {
			span.RecordError(err)
			errorList = errors.Join(errorList, err)
			contextLogger.Error(err, "timeline history upload failed",
				"tli", tli, "file", result.FileName)

			continue
		}

		contextLogger.Info("Stored history file", "timeline", tli, "fileName", result.FileName)
	}

	return errorList
}

func (s *Process) startReplication(
	ctx context.Context,
	conn *pgconn.PgConn,
	coordinate *walCoordinate,
	walSegmentSize uint64,
) error {
	contextLogger := log.FromContext(ctx)

	startXlog := coordinate.lsn
	timeline := coordinate.timeline

	for {
		// To find the replication start position, we go back to the start of the WAL file
		startWalLSNString, err := types.Int64ToLSN(uint64(startXlog)).WALFileStart(walSegmentSize)
		if err != nil {
			return fmt.Errorf("while computing the LSN of the WAL start - shift: %w", err)
		}

		startWalLSN, err := startWalLSNString.Parse()
		if err != nil {
			return fmt.Errorf("while computing the LSN of the WAL start - parse: %w", err)
		}

		startXLogPos := pglogrepl.LSN(startWalLSN)

		err = pglogrepl.StartReplication(
			ctx,
			conn,
			s.options.Slot,
			startXLogPos,
			pglogrepl.StartReplicationOptions{
				Timeline: timeline,
				Mode:     pglogrepl.PhysicalReplication,
			})
		if err != nil {
			return fmt.Errorf("while running start_replication: %w", err)
		}

		contextLogger.Info(
			"Physical replication started",
			"slotName", s.options.Slot,
			"startWalLSN", startWalLSN,
			"timeline", timeline,
		)

		handler := s.newHandler(int(timeline), walSegmentSize)

		walBuffer := buffer.New(
			int(timeline),
			walSegmentSize,
			handler,
			s.options.BufferSize,
		)

		copyDoneResult, err := s.manageWALStream(ctx, conn, walBuffer)
		if err != nil {
			return err
		}

		if handler.HasWALFileOpened() {
			// If the transmission terminated but there is still a WAL file in progress,
			// we close it.
			// This happens when PG is shut down.
			if err := handler.CloseWAL(ctx); err != nil {
				return fmt.Errorf("while closing the WAL file: %w", err)
			}
		}

		// Check if the timeline has changed and restart replication if needed
		if copyDoneResult != nil && copyDoneResult.Timeline != timeline {
			contextLogger.Info(
				"Timeline changed, restarting replication",
				"oldTimeline", timeline,
				"newTimeline", copyDoneResult.Timeline,
				"newStartLSN", copyDoneResult.LSN,
			)

			// Update timeline and starting position for restart.
			timeline = copyDoneResult.Timeline
			startXlog = copyDoneResult.LSN

			// Continue the loop to restart replication with the new timeline
			continue
		}

		// If we reach here, replication completed without timeline change
		break
	}

	return nil
}

//nolint:gocognit,cyclop
func (s *Process) manageWALStream(
	ctx context.Context,
	conn *pgconn.PgConn,
	buffer *buffer.Data,
) (*pglogrepl.CopyDoneResult, error) {
	contextLogger := log.FromContext(ctx)

	flushDeadline := s.options.FlushTimeout
	nextFlushDeadline := time.Now().Add(flushDeadline)

	feedbackDeadline := s.options.StandbyMessageTimeout
	nextFeedbackDeadline := time.Now().Add(feedbackDeadline)

loop:
	for {
		if time.Now().After(nextFlushDeadline) {
			flushedLSN := buffer.FlushLSN()

			if err := buffer.Flush(ctx); err != nil {
				contextLogger.Error(err, "Failed flush WAL data")
				return nil, fmt.Errorf("while flushing WAL data: %w", err)
			}

			// When flush really wrote something down to the destination,
			// the FlushedLSN will be different. In that case, we want to
			// immediately give feedback to the PostgreSQL server. This
			// ultimately will result in updated data in pg_stat_replication.
			if flushedLSN != buffer.FlushLSN() {
				nextFeedbackDeadline = time.Time{}
			}

			nextFlushDeadline = time.Now().Add(flushDeadline)
		}

		if time.Now().After(nextFeedbackDeadline) {
			// We communicate back to PostgreSQL the feedback when:
			//
			// 1. the feedback deadline exceeded
			// 2. we received something from streaming replication
			s.sendFeedback(ctx, conn, buffer)
			nextFeedbackDeadline = time.Now().Add(feedbackDeadline)
		}

		standbyMessageDeadlineContext, cancel := context.WithDeadline(ctx, nextFlushDeadline)
		msg, err := conn.ReceiveMessage(standbyMessageDeadlineContext)
		cancel()

		if err != nil {
			if pgconn.Timeout(err) {
				continue
			}
			if errors.Is(err, context.Canceled) {
				break
			}
			contextLogger.Error(err, "receive message failed")

			break
		}

		log.FromContext(ctx).Trace(
			"Received message",
			"msgType", fmt.Sprintf("%T", msg))

		switch msg := msg.(type) {
		case *pgproto3.CopyData:
			switch msg.Data[0] {
			case pglogrepl.PrimaryKeepaliveMessageByteID:
				pkm, err := pglogrepl.ParsePrimaryKeepaliveMessage(msg.Data[1:])
				if err != nil {
					contextLogger.Error(err, "parsePrimaryKeepaliveMessage failed")
					continue
				}
				contextLogger.Debug(
					"Primary Keepalive Message",
					"ServerWALEnd", pkm.ServerWALEnd,
					"ServerTime", pkm.ServerTime,
					"ReplyRequested", pkm.ReplyRequested,
				)

				if pkm.ReplyRequested {
					s.sendFeedback(ctx, conn, buffer)
				}

			case pglogrepl.XLogDataByteID:
				xld, err := pglogrepl.ParseXLogData(msg.Data[1:])
				if err != nil {
					contextLogger.Error(err, "ParseXLogData failed")
					continue
				}

				err = buffer.ProcessWALData(ctx, xld.WALData, types.LSN(xld.WALStart.String()))
				if err != nil {
					contextLogger.Error(err, "Error while processing WAL data", "lsn", xld.WALStart)

					return nil, fmt.Errorf("could not process WAL data at %s: %w", xld.WALStart, err)
				}

				// Force the code to communicate back to PostgreSQL the current status without waiting for
				// a flush
				nextFeedbackDeadline = time.Time{}

			default:
				contextLogger.Info("Received unexpected copydata message", "msg", msg)
				return nil, NewUnexpectedCopydataMessageError(msg.Data)
			}

		case *pgproto3.CommandComplete:
			contextLogger.Info("Streaming replication terminated by the backend with success")
			return nil, nil

		case *pgproto3.CopyDone:
			contextLogger.Info("Streaming replication terminated by the backend with CopyDone")
			break loop

		default:
			contextLogger.Info("Received unexpected message", "msg", msg)
			return nil, NewUnexpectedMessageError(msg)
		}
	}

	contextLogger.Info("WAL streaming loop terminated, sending CopyDone")
	copyDoneResult, err := pglogrepl.SendStandbyCopyDone(ctx, conn)
	if err != nil {
		return nil, fmt.Errorf("failed to send CopyDone message: %w", err)
	}

	contextLogger.Info(
		"Physical replication finished",
		"timeline", copyDoneResult.Timeline,
		"lsn", copyDoneResult.LSN,
	)

	return copyDoneResult, nil
}

func (s *Process) sendFeedback(ctx context.Context, conn *pgconn.PgConn, buffer *buffer.Data) {
	contextLogger := log.FromContext(ctx)

	err := pglogrepl.SendStandbyStatusUpdate(
		ctx,
		conn,
		pglogrepl.StandbyStatusUpdate{
			WALWritePosition: pglogrepl.LSN(buffer.WriteLSN()),
			WALFlushPosition: pglogrepl.LSN(buffer.FlushLSN()),
			WALApplyPosition: pglogrepl.LSN(buffer.FlushLSN()),
		},
	)
	if err != nil {
		contextLogger.Error(err, "Failed to send standby status update, skipping")
	} else {
		contextLogger.Debug(
			"Sent Standby status message",
			"write_lsn", types.Int64ToLSN(buffer.WriteLSN()),
			"flush_lsn", types.Int64ToLSN(buffer.FlushLSN()))
	}
}
