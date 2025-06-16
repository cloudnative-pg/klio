package sendwal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/types"
	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"

	"github.com/EnterpriseDB/klio/internal/client/klioclient/grpcclient"
	"github.com/EnterpriseDB/klio/internal/client/sendwal/buffer"
	"github.com/EnterpriseDB/klio/internal/client/sendwal/infrastructure"
	klioGRPC "github.com/EnterpriseDB/klio/internal/grpc"
	"github.com/EnterpriseDB/klio/pkg/config"
)

// Process implements the WAL sender service.
type Process struct {
	config         *config.Data
	logger         *slog.Logger
	infrastructure *infrastructure.Postgres
	client         *grpcclient.Connection
}

// New creates a new receiver.
func New(cfg *config.Data, log *slog.Logger, client *grpcclient.Connection) *Process {
	return &Process{
		config:         cfg,
		logger:         log.With("service", "receive_wal"),
		infrastructure: infrastructure.NewPostgres(cfg, log),
		client:         client,
	}
}

// ResetReplicationStatus reset the replication status on the server side and then
// drops the Klio replication slot.
func (s *Process) ResetReplicationStatus(
	ctx context.Context,
) error {
	conn, err := s.infrastructure.NewConn(ctx)
	if err != nil {
		return fmt.Errorf("while parsing DSN: %w", err)
	}
	defer func() {
		if closeErr := conn.Close(ctx); closeErr != nil {
			s.logger.ErrorContext(ctx, "Error while closing the connection")
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

	s.logger.Info(
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

	s.logger.Info(
		"Dropped replication slot",
		"name", slotName)

	return nil
}

// Start the WAL receiver.
func (s *Process) Start(ctx context.Context) error {
	conn, err := s.infrastructure.NewConn(ctx)
	if err != nil {
		return fmt.Errorf("while parsing DSN: %w", err)
	}
	defer func() {
		if closeErr := conn.Close(ctx); closeErr != nil {
			s.logger.ErrorContext(ctx, "Error while closing the connection")
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

	s.logger.Info(
		"Current system identification data",
		"xlogFlushPosition", identifyData.XLogPos,
		"timeline", identifyData.Timeline,
		"systemID", identifyData.SystemID,
	)

	startingPoint, err := s.getReplicationStartPoint(ctx, conn, identifyData, walSegmentSize)
	if err != nil {
		return err
	}

	if err := s.downloadHistoryFiles(ctx, conn, identifyData.Timeline); err != nil {
		return fmt.Errorf("while downloading history files: %w", err)
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
	// Find the latest replication point reading the replication slot
	slotResult, err := ReadReplicationSlot(ctx, conn, s.config.Source.Slot)
	if err != nil {
		return 0, fmt.Errorf("while reading replication slot: %w", err)
	}
	if slotResult.RestartLSN != 0 {
		startPoint := pglogrepl.LSN(uint64(slotResult.RestartLSN) & ^(segmentSize - 1))
		s.logger.Debug(
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
	s.logger.Debug(
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

	serverWALFileName, err := s.client.RequestWALStart(ctx, opts)
	if err != nil {
		return 0, fmt.Errorf("during server-side replication point validation: %w", err)
	}

	lsn, err := getReplicationStartFromWALFileName(serverWALFileName.GetWalName(), segmentSize)
	if err != nil {
		return 0, err
	}

	s.logger.Info(
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

	s.logger.Info(
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
	for tli := currentTli; tli > 1; tli-- {
		result, err := pglogrepl.TimelineHistory(ctx, conn, tli)
		if err != nil {
			return fmt.Errorf("while downloading history for timeline %v: %w", tli, err)
		}

		if err := s.client.StoreHistoryFile(ctx, result.FileName, result.Content); err != nil {
			return fmt.Errorf("while uploading history file: %w", err)
		}
	}

	return nil
}

func (s *Process) startReplication(
	ctx context.Context,
	conn *pgconn.PgConn,
	startXlog pglogrepl.LSN,
	timeline int32,
	walSegmentSize uint64,
) error {
	// To find the replication start position, we go back to the start of the WAL file
	startWalLSNString, err := types.Int64ToLSN(uint64(startXlog)).WALFileStart(walSegmentSize)
	if err != nil {
		return fmt.Errorf("while computing the LSN of the WAL start - shift: %w", err)
	}

	startWalLSN, err := startWalLSNString.Parse()
	if err != nil {
		return fmt.Errorf("while computing the LSN of the WAL start - parse: %w", err)
	}

	clientXLogPos := pglogrepl.LSN(startWalLSN)

	err = pglogrepl.StartReplication(
		ctx,
		conn,
		s.config.Source.Slot,
		clientXLogPos,
		pglogrepl.StartReplicationOptions{
			Timeline: timeline,
			Mode:     pglogrepl.PhysicalReplication,
		})
	if err != nil {
		return fmt.Errorf("while running start_replication: %w", err)
	}

	s.logger.Info(
		"Physical replication started",
		"slotName",
		s.config.Source.Slot,
		"startWalLSN",
		startWalLSN,
	)

	klioHandler := buffer.NewKlioClientHandler(
		s.logger,
		int(timeline),
		walSegmentSize,
		s.client,
	)

	buffer := buffer.New(
		s.logger,
		int(timeline),
		walSegmentSize,
		klioHandler,
	)

	if err := s.manageWALStream(ctx, conn, clientXLogPos, buffer); err != nil {
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
	clientXLogPos pglogrepl.LSN,
	buffer *buffer.Data,
) error {
	standbyMessageTimeout := s.config.Source.StandbyMessageTimeout()
	nextStandbyMessageDeadline := time.Now().Add(standbyMessageTimeout)

	for {
		if time.Now().After(nextStandbyMessageDeadline) {
			err := pglogrepl.SendStandbyStatusUpdate(
				ctx,
				conn,
				pglogrepl.StandbyStatusUpdate{
					WALWritePosition: clientXLogPos,
				},
			)
			if err != nil {
				s.logger.Error("Failed to send standby status update, skipping", "err", err)
			} else {
				s.logger.Debug("Sent Standby status message", "xlogPos", clientXLogPos)
			}
			nextStandbyMessageDeadline = time.Now().Add(standbyMessageTimeout)
		}

		standbyMessageDeadlineContext, cancel := context.WithDeadline(ctx, nextStandbyMessageDeadline)
		msg, err := conn.ReceiveMessage(standbyMessageDeadlineContext)
		cancel()

		if err != nil {
			if pgconn.Timeout(err) {
				continue
			}
			if errors.Is(err, context.Canceled) {
				break
			}
			s.logger.Error("Receive message failed", "err", err)

			break
		}

		switch msg := msg.(type) {
		case *pgproto3.CopyData:
			switch msg.Data[0] {
			case pglogrepl.PrimaryKeepaliveMessageByteID:
				pkm, err := pglogrepl.ParsePrimaryKeepaliveMessage(msg.Data[1:])
				if err != nil {
					s.logger.Error("ParsePrimaryKeepaliveMessage failed", "err", err)
					continue
				}
				s.logger.Debug(
					"Primary Keepalive Message",
					"ServerWALEnd", pkm.ServerWALEnd,
					"ServerTime", pkm.ServerTime,
					"ReplyRequested", pkm.ReplyRequested,
				)

				if pkm.ReplyRequested {
					nextStandbyMessageDeadline = time.Time{}
				}

			case pglogrepl.XLogDataByteID:
				xld, err := pglogrepl.ParseXLogData(msg.Data[1:])
				if err != nil {
					s.logger.Error("ParseXLogData failed", "err", err)
					continue
				}

				if err = buffer.ProcessWALData(ctx, xld.WALData, types.LSN(xld.WALStart.String())); err != nil {
					s.logger.Error("Error while processing WAL data", "err", err, "lsn", xld.WALStart)
					return fmt.Errorf("could not process WAL data at %s: %w", xld.WALStart, err)
				}

				clientXLogPos = xld.WALStart + pglogrepl.LSN(len(xld.WALData))

			default:
				s.logger.Info("Received unexpected copydata message", "msg", msg)
				return NewUnexpectedCopydataMessageError(msg.Data)
			}

		case *pgproto3.CommandComplete:
			s.logger.Info("Streaming replication terminated by the backend with success")
			return nil

		default:
			s.logger.Info("Received unexpected message", "msg", msg)
			return NewUnexpectedMessageError(msg)
		}
	}

	copyDoneResult, err := pglogrepl.SendStandbyCopyDone(ctx, conn)
	if err != nil {
		return fmt.Errorf("failed to send CopyDone message: %w", err)
	}

	s.logger.Info(
		"Physical replication finished",
		"timeline", copyDoneResult.Timeline,
		"lsn", copyDoneResult.LSN,
	)

	return nil
}
