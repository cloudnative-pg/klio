package walserver

import (
	"context"
	"sync/atomic"

	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/grpc"
)

// feedbackSenderToKlioClient decouples sending WAL Put progress acks to the client from
// the caller's write/flush cadence. SetWrittenSize never blocks on network
// I/O: it just records the latest known size, since writtenSize is a
// monotonically increasing cumulative total and therefore always
// supersedes an older, unsent value. A background goroutine wakes up on
// each update and sends that latest size, so a batch of blocks written and
// flushed together produces a single ack instead of one per block.
//
// pending is never reset after being sent: it always holds "the latest
// known size", so a wake-up with nothing new to report (e.g. Stop firing
// right after a send already carried the latest size) just resends that
// same value - redundant, but harmless, since only the latest value is
// ever meaningful to the client. 0 is the one value treated as "nothing to
// report yet" and is never sent.
//
// req.Send is not safe for concurrent use: Stop must be called, and must
// return, before the caller uses req.Send again directly - e.g. before
// finalize sends the closing PutResult.
type feedbackSenderToKlioClient struct {
	writeLSN atomic.Uint64
	flushLSN atomic.Uint64

	wake   chan struct{}
	cancel context.CancelFunc
	done   chan struct{}
}

// newFeedbackSenderToKlioClient starts the background send loop for req.
func newFeedbackSenderToKlioClient(ctx context.Context, req grpc.WAL_PutServer) *feedbackSenderToKlioClient {
	ctx, cancel := context.WithCancel(ctx)
	s := &feedbackSenderToKlioClient{
		wake:   make(chan struct{}, 1),
		cancel: cancel,
		done:   make(chan struct{}),
	}

	go s.run(ctx, req)

	return s
}

// SetWriteLSN queues write_lsn to be acked.
func (s *feedbackSenderToKlioClient) SetFeedback(writeLSN uint64, flushLSN uint64) {
	if writeLSN == 0 || flushLSN == 0 {
		return
	}

	s.writeLSN.Store(writeLSN)
	s.flushLSN.Store(flushLSN)

	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Stop cancels the background goroutine, which flushes any size queued via
// SetWrittenSize but not yet sent before it exits, then waits for it to do
// so. Safe to call more than once.
func (s *feedbackSenderToKlioClient) Stop() {
	s.cancel()
	<-s.done
}

func (s *feedbackSenderToKlioClient) run(ctx context.Context, req grpc.WAL_PutServer) {
	defer close(s.done)

	for {
		select {
		case <-s.wake:
			s.sendLatest(ctx, req)
		case <-ctx.Done():
			s.sendLatest(ctx, req)
			return
		}
	}
}

func (s *feedbackSenderToKlioClient) sendLatest(ctx context.Context, req grpc.WAL_PutServer) {
	if err := req.Send(
		&grpc.PutResult{
			WriteLsn: s.writeLSN.Load(),
			FlushLsn: s.flushLSN.Load(),
		},
	); err != nil {
		log.FromContext(ctx).Error(
			err,
			"Error while sending feedback to the client, skipping",
		)
	}
}
