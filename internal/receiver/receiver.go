// Package receiver implements the receive_wal service
package receiver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/types"
	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"

	"github.com/EnterpriseDB/klio/internal/infrastructure"
	"github.com/EnterpriseDB/klio/internal/receiver/buffer"
	"github.com/EnterpriseDB/klio/pkg/config"
	"github.com/EnterpriseDB/klio/pkg/klioclient"
)

// Process implements the supervisor service.
type Process struct {
	config         *config.Data
	logger         *slog.Logger
	infrastructure *infrastructure.Postgres
	client         *klioclient.Connection
}

// New creates a new receiver.
func New(cfg *config.Data, log *slog.Logger, client *klioclient.Connection) *Process {
	return &Process{
		config:         cfg,
		logger:         log.With("service", "receive_wal"),
		infrastructure: infrastructure.NewPostgres(cfg, log),
		client:         client,
	}
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

	//nolint:godox
	// TODO(leonardoce): create a physical replication slot and reuse it
	// to keep track of the latest LSN
	replicationSlotResult, err := pglogrepl.CreateReplicationSlot(
		ctx,
		conn,
		s.config.Source.Slot,
		"", // output plugin name: this is meaningful only for physical replication
		pglogrepl.CreateReplicationSlotOptions{
			//nolint:godox
			// TODO: replication slot should not be temporary. It also should
			// be copied on standbys, since we can't afford not finding WALs.
			Temporary: true,
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

	walSegmentSize, err := s.infrastructure.GetWalSegmentSize(ctx)
	if err != nil {
		return fmt.Errorf("while setting up replication: %w", err)
	}

	return s.startReplication(ctx, conn, identifyData.XLogPos, identifyData.Timeline, walSegmentSize)
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
	buffer := buffer.New(
		s.logger,
		int(timeline),
		walSegmentSize,
		func(walName string, data []byte) error {
			s.logger.Info("Archiving WAL", "walName", walName, "size", len(data))
			return s.client.StoreWAL(ctx, walName, data)
		},
	)

	if err := s.manageWALStream(ctx, conn, clientXLogPos, buffer); err != nil {
		return err
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

				if err = buffer.ProcessWALData(
					xld.WALData,
					types.LSN(xld.WALStart.String()),
				); err != nil {
					s.logger.Error("Error while processing WAL data", "err", err, "lsn", xld.WALStart)
					return fmt.Errorf("could not process WAL data at %s: %w", xld.WALStart, err)
				}

				clientXLogPos = xld.WALStart + pglogrepl.LSN(len(xld.WALData))

			default:
				s.logger.Info("Received unexpected copydata message", "msg", msg)
				return NewUnexpectedCopydataMessageError(msg.Data)
			}
		default:
			s.logger.Info("Received unexpected message", "msg", msg)
		}
	}

	return nil
}
