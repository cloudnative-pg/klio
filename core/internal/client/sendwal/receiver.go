package sendwal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/cloudnative-pg/machinery/pkg/types"
	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"
	"go.uber.org/multierr"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/grpcclient"
	"github.com/cloudnative-pg/klio/core/internal/client/sendwal/buffer"
	"github.com/cloudnative-pg/klio/core/internal/client/sendwal/infrastructure"
	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// Process implements the WAL sender service.
type Process struct {
	config         *config.Data
	infrastructure *infrastructure.Postgres
	client         *grpcclient.Connection
}

// New creates a new receiver.
func New(cfg *config.Data, logger log.Logger, client *grpcclient.Connection) *Process {
	return &Process{
		config:         cfg,
		infrastructure: infrastructure.NewPostgres(cfg, logger),
		client:         client,
	}
}

// ResetReplicationStatus reset the replication status on the server side and then
// drops the Klio replication slot.
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

	result, err := s.client.ResetWALStream(ctx, &klioGRPC.ResetWALStreamRequest{
		ClusterName:    s.config.Client.Wal.ClusterName,
		SystemId:       identifyData.SystemID,
		CurrentWalName: clientWALFileName,
	})
	if err != nil {
		return fmt.Errorf("while invoking server-side replication reset: %w", err)
	}

	contextLogger.Info(
		"Reset server-side replication status",
		"walName", result)

	slotName := s.config.Source.Slot
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

	startingPoint, err := s.getReplicationStartPoint(ctx, conn, identifyData, walSegmentSize)
	if err != nil {
		return err
	}

	// We cannot guarantee to have all the history files available, so we ignore the error.
	// The wal receiver could have been configured later in the cluster lifecycle
	if histErr := s.downloadHistoryFiles(ctx, conn, identifyData.Timeline); histErr != nil {
		contextLogger.Debug("Some timeline history files could not be processed", "innerErr", histErr.Error())
	}

	if err := s.ensureReplicationSlotExists(ctx, conn); err != nil {
		return err
	}

	return s.startReplication(ctx, conn, startingPoint, identifyData.Timeline, walSegmentSize)
}

func (s *Process) getReplicationStartPointFromClient(
	ctx context.Context,
	conn *pgconn.PgConn,
	xlogFlushPos pglogrepl.LSN,
	segmentSize uint64,
) (pglogrepl.LSN, error) {
	contextLogger := log.FromContext(ctx)

	// Find the latest replication point reading the replication slot
	slotResult, err := ReadReplicationSlot(ctx, conn, s.config.Source.Slot)
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

	// If nor the Klio server nor the replication slot are set,
	// we use the XLOG flush position, taking care of
	// starting streaming from the beginning of the WAL file.
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

func (s *Process) getReplicationStartPoint(
	ctx context.Context,
	conn *pgconn.PgConn,
	data pglogrepl.IdentifySystemResult,
	segmentSize uint64,
) (pglogrepl.LSN, error) {
	contextLogger := log.FromContext(ctx)

	clientStartLSN, err := s.getReplicationStartPointFromClient(ctx, conn, data.XLogPos, segmentSize)
	if err != nil {
		return 0, err
	}

	clientWALFileName, err := types.Int64ToLSN(uint64(clientStartLSN)).WALFileName(int(data.Timeline), segmentSize)
	if err != nil {
		return 0, fmt.Errorf("while converting LSN to WAL file name: %q %w", clientStartLSN, err)
	}

	opts := &klioGRPC.RequestWALStartRequest{
		ClusterName:    s.config.Client.Wal.ClusterName,
		SystemId:       data.SystemID,
		CurrentWalName: clientWALFileName,
	}

	contextLogger.Info("requesting server-side replication start", "opts", opts)

	serverWALFileName, err := s.client.RequestWALStart(ctx, opts)
	if err != nil {
		return 0, fmt.Errorf("during server-side replication point validation: %w", err)
	}

	lsn, err := getReplicationStartFromWALFileName(serverWALFileName.GetWalName(), segmentSize)
	if err != nil {
		return 0, err
	}

	contextLogger.Info(
		"Negotiated replication start with the server",
		"lsn", lsn)

	return lsn, nil
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

	slotResult, err := ReadReplicationSlot(ctx, conn, s.config.Source.Slot)
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
		s.config.Source.Slot,
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

	contextLogger := log.FromContext(ctx)
	for tli := currentTli; tli > 1; tli-- {
		result, err := pglogrepl.TimelineHistory(ctx, conn, tli)
		if err != nil {
			contextLogger.Error(err, "timeline history fetching failed, skipping", "tli", tli)
			errorList = multierr.Append(errorList, err)

			continue
		}

		if err := s.client.StoreHistoryFile(ctx, result.FileName, result.Content); err != nil {
			errorList = multierr.Append(errorList, err)
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
	startXlog pglogrepl.LSN,
	timeline int32,
	walSegmentSize uint64,
) error {
	contextLogger := log.FromContext(ctx)

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
		s.config.Source.Slot,
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
		"slotName", s.config.Source.Slot,
		"startWalLSN", startWalLSN,
	)

	klioHandler := buffer.NewKlioClientHandler(
		int(timeline),
		walSegmentSize,
		s.client,
	)

	walBuffer := buffer.New(
		int(timeline),
		walSegmentSize,
		klioHandler,
		s.config.Source.BufferSize,
	)

	if err := s.manageWALStream(ctx, conn, walBuffer); err != nil {
		return err
	}

	if klioHandler.HasWALFileOpened() {
		// If the transmission terminated but there is still a WAL file in progress,
		// we close it.
		// This happens when PG is shut down.
		if err := klioHandler.CloseWAL(ctx); err != nil {
			return fmt.Errorf("while closing the WAL file: %w", err)
		}
	}

	if err := conn.Close(ctx); err != nil {
		return fmt.Errorf("while closing connection: %w", err)
	}

	return nil
}

//nolint:gocognit,cyclop
func (s *Process) manageWALStream(
	ctx context.Context,
	conn *pgconn.PgConn,
	buffer *buffer.Data,
) error {
	contextLogger := log.FromContext(ctx)

	flushDeadline := s.config.Source.FlushTimeout()
	nextFlushDeadline := time.Now().Add(flushDeadline)

	feedbackDeadline := s.config.Source.StandbyMessageTimeout()
	nextFeedbackDeadline := time.Now().Add(feedbackDeadline)

	for {
		if time.Now().After(nextFlushDeadline) {
			flushedLSN := buffer.FlushLSN()
			if err := buffer.Flush(ctx); err != nil {
				contextLogger.Error(err, "Failed flush WAL data")
				return fmt.Errorf("while flushing WAL data: %w", err)
			}

			// TODO: explain why it can differ
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

				if err = buffer.ProcessWALData(ctx, xld.WALData, types.LSN(xld.WALStart.String())); err != nil {
					contextLogger.Error(err, "Error while processing WAL data", "lsn", xld.WALStart)
					return fmt.Errorf("could not process WAL data at %s: %w", xld.WALStart, err)
				}

				// Force the code to communicate back to PostgreSQL the current status without waiting for
				// a flush
				nextFeedbackDeadline = time.Time{}

			default:
				contextLogger.Info("Received unexpected copydata message", "msg", msg)
				return NewUnexpectedCopydataMessageError(msg.Data)
			}

		case *pgproto3.CommandComplete:
			contextLogger.Info("Streaming replication terminated by the backend with success")
			return nil

		default:
			contextLogger.Info("Received unexpected message", "msg", msg)
			return NewUnexpectedMessageError(msg)
		}
	}

	copyDoneResult, err := pglogrepl.SendStandbyCopyDone(ctx, conn)
	if err != nil {
		return fmt.Errorf("failed to send CopyDone message: %w", err)
	}

	contextLogger.Info(
		"Physical replication finished",
		"timeline", copyDoneResult.Timeline,
		"lsn", copyDoneResult.LSN,
	)

	return nil
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
