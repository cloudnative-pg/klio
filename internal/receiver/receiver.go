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
	"github.com/thejerf/suture/v4"

	"github.com/EnterpriseDB/klio/internal/receiver/buffer"
	"github.com/EnterpriseDB/klio/internal/tier1"
	"github.com/EnterpriseDB/klio/pkg/config"
)

// Service represent the WAL receiver service
type Service interface {
	suture.Service
}

// Service implements the supervisor service
type impl struct {
	Config *config.Data
	Logger *slog.Logger
	Tier1  tier1.Service
}

// New creates a new receiver
func New(config *config.Data, logger *slog.Logger, t1 tier1.Service) Service {
	return &impl{
		Config: config,
		Logger: logger.With("service", "receive_wal"),
		Tier1:  t1,
	}
}

// String implements the Stringer interface and is used for logging
func (s *impl) String() string {
	return "receive_wal"
}

// Serve is the implementation of the service and is called when
// the WAL receiver is attached to the supervisor
func (s *impl) Serve(ctx context.Context) error {
	if !s.Tier1.IsReady() {
		return errors.New("tier1 service is not ready")
	}

	conn, err := pgconn.Connect(ctx, s.Config.Source.DSN)
	if err != nil {
		return fmt.Errorf("while parsing DSN: %w", err)
	}
	defer func() {
		if closeErr := conn.Close(ctx); closeErr != nil {
			s.Logger.ErrorContext(ctx, "Error while closing the connection")
		}
	}()

	identifyData, err := pglogrepl.IdentifySystem(ctx, conn)
	if err != nil {
		return fmt.Errorf("while executing identify_system: %w", err)
	}

	s.Logger.Info(
		"Current system identification data",
		"xlogFlushPosition", identifyData.XLogPos,
		"timeline", identifyData.Timeline,
		"systemID", identifyData.SystemID,
	)

	walSegmentSize, err := s.getWALSegmentSize(ctx, conn)
	if err != nil {
		return fmt.Errorf("while getting WAL segment size: %w", err)
	}
	s.Logger.Info(
		"Detected WAL segment size",
		"walSegmentSize", walSegmentSize,
	)

	// TODO(leonardoce): create a physical replication slot and reuse it
	// to keep track of the latest LSN
	replicationSlotResult, err := pglogrepl.CreateReplicationSlot(
		ctx,
		conn,
		s.Config.Source.Slot,
		"", // output plugin name: this is meaningful only for physical replication
		pglogrepl.CreateReplicationSlotOptions{
			Temporary: true,
			Mode:      pglogrepl.PhysicalReplication,
		},
	)
	if err != nil {
		return fmt.Errorf("while creating temporary replication slot: %w", err)
	}

	s.Logger.Info(
		"Created replication slot",
		"consistentPoint", replicationSlotResult.ConsistentPoint,
		"name", replicationSlotResult.SlotName)

	return s.startReplication(ctx, conn, identifyData.XLogPos, identifyData.Timeline, walSegmentSize)
}

func (s *impl) startReplication(
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
		s.Config.Source.Slot,
		clientXLogPos,
		pglogrepl.StartReplicationOptions{
			Timeline: timeline,
			Mode:     pglogrepl.PhysicalReplication,
		})
	if err != nil {
		return fmt.Errorf("while running start_replication: %w", err)
	}

	s.Logger.Info(
		"Physical replication started",
		"slotName",
		s.Config.Source.Slot,
		"startWalLSN",
		startWalLSN,
	)
	buff := buffer.New(
		s.Logger,
		int(timeline),
		walSegmentSize,
		func(walName string, data []byte) error {
			s.Logger.Info("Archiving WAL", "walName", walName, "size", len(data))
			return s.Tier1.StoreWAL(ctx, walName, data)
		},
	)

	if err := s.manageWALStream(ctx, conn, clientXLogPos, buff); err != nil {
		return err
	}

	copyDoneResult, err := pglogrepl.SendStandbyCopyDone(ctx, conn)
	if err != nil {
		return fmt.Errorf("failed to send CopyDone message: %w", err)
	}

	s.Logger.Info(
		"Physical replication finished",
		"timeline", copyDoneResult.Timeline,
		"lsn", copyDoneResult.LSN,
	)

	return nil
}

// menageWALStream requires that a replication has been started on the passed conn
//
//nolint:gocognit
func (s *impl) manageWALStream(
	ctx context.Context,
	replicationConn *pgconn.PgConn,
	clientXLogPos pglogrepl.LSN,
	buffer *buffer.Data,
) error {
	standbyMessageTimeout := s.Config.Source.StandbyMessageTimeout()
	nextStandbyMessageDeadline := time.Now().Add(standbyMessageTimeout)

	for {
		if time.Now().After(nextStandbyMessageDeadline) {
			err := pglogrepl.SendStandbyStatusUpdate(
				ctx,
				replicationConn,
				pglogrepl.StandbyStatusUpdate{
					WALWritePosition: clientXLogPos,
				},
			)
			if err != nil {
				s.Logger.Error("Failed to send standby status update, skipping", "err", err)
			} else {
				s.Logger.Debug("Sent Standby status message", "xlogPos", clientXLogPos)
			}

			s.Logger.Debug("refreshing nextStandbyMessageDeadline")
			nextStandbyMessageDeadline = time.Now().Add(standbyMessageTimeout)
		}

		standbyMessageDeadlineContext, cancel := context.WithDeadline(ctx, nextStandbyMessageDeadline)
		// we fetch the messages from the replication connection
		backendMessage, err := replicationConn.ReceiveMessage(standbyMessageDeadlineContext)
		cancel()

		if err != nil {
			if pgconn.Timeout(err) {
				continue
			}
			if errors.Is(err, context.Canceled) {
				break
			}
			s.Logger.Error("Receive message failed", "err", err)
			break
		}

		switch msg := backendMessage.(type) {
		case *pgproto3.CopyData:
			payloadID := msg.Data[0]
			switch payloadID {
			case pglogrepl.PrimaryKeepaliveMessageByteID:
				pkm, err := pglogrepl.ParsePrimaryKeepaliveMessage(msg.Data[1:])
				if err != nil {
					s.Logger.Error("ParsePrimaryKeepaliveMessage failed", "err", err)
					continue
				}
				s.Logger.Debug(
					"Primary Keepalive Message",
					"ServerWALEnd", pkm.ServerWALEnd,
					"ServerTime", pkm.ServerTime,
					"ReplyRequested", pkm.ReplyRequested,
				)

				if pkm.ReplyRequested {
					// we trigger a reply to the primary at the beginning of the next loop
					nextStandbyMessageDeadline = time.Time{}
				}

			case pglogrepl.XLogDataByteID:
				xld, err := pglogrepl.ParseXLogData(msg.Data[1:])
				if err != nil {
					s.Logger.Error("ParseXLogData failed", "err", err)
					continue
				}

				if err = buffer.ProcessWALData(
					xld.WALData,
					types.LSN(xld.WALStart.String()),
				); err != nil {
					s.Logger.Error("Error while processing WAL data", "err", err, "lsn", xld.WALStart)
					return err
				}

				clientXLogPos = xld.WALStart + pglogrepl.LSN(len(xld.WALData))

			default:
				s.Logger.Info("Received unexpected copydata message", "msg", msg)
				return fmt.Errorf("received unexpected copydata message type", msg)
			}
		default:
			s.Logger.Info("Received unexpected message", "msg", msg)
			return fmt.Errorf("received unexpected message type: %v", msg)
		}
	}
	return nil
}
