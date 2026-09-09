package walserver

import (
	"context"
	"io"

	"github.com/cloudnative-pg/klio/core/internal/grpc"
)

// blockReceiverBufferSize bounds how many blocks can be queued ahead of the
// caller before the background goroutine blocks trying to enqueue another
// one.
const blockReceiverBufferSize = 500

// blockReceiver decouples receiving WAL blocks from the client's send
// cadence. A background goroutine blocks on req.Recv in a tight loop and
// publishes every block on a channel, so Drain can collect however many
// blocks are already queued on the wire without an artificial wait -
// batching them into a single write+flush instead of one fsync per block.
//
// req.Recv is not safe for concurrent use, so the background goroutine owns
// it exclusively: nothing else may call req.Recv for the lifetime of this
// blockReceiver.
type blockReceiver struct {
	ch     chan *grpc.PutRequest
	cancel context.CancelFunc
	done   chan struct{}

	// err is set by the background goroutine, once, before it closes ch.
	err error
}

// newBlockReceiverFromKlioClient starts the background receive loop for req.
func newBlockReceiverFromKlioClient(ctx context.Context, req grpc.WAL_PutServer) *blockReceiver {
	ctx, cancel := context.WithCancel(ctx)
	r := &blockReceiver{
		ch:     make(chan *grpc.PutRequest, blockReceiverBufferSize),
		cancel: cancel,
		done:   make(chan struct{}),
	}

	go r.run(ctx, req)

	return r
}

// Drain returns every block currently queued, blocking only if none are
// available yet. Once the stream ends, a non-nil error is only ever
// returned together with a nil batch - io.EOF marks a clean end of stream,
// any other error a failed read. Any blocks still buffered when the stream
// ends are returned first, with a nil error; the closed channel stays
// closed, so the very next call sees it immediately and returns the error.
func (r *blockReceiver) Drain(ctx context.Context) ([]*grpc.PutRequest, error) {
	var batch []*grpc.PutRequest

	select {
	case request, ok := <-r.ch:
		if !ok {
			return nil, r.closedErr()
		}
		batch = append(batch, request)
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	for {
		select {
		case request, ok := <-r.ch:
			if !ok {
				return batch, nil
			}
			batch = append(batch, request)
		default:
			return batch, nil
		}
	}
}

// Cancel signals the background goroutine to stop, without waiting for it
// to actually exit. Safe to call even while req.Recv is blocked waiting for
// more data from the client - see the "early-exit" note on Stop.
func (r *blockReceiver) Cancel() {
	r.cancel()
}

// Stop cancels the background receive loop and waits for it to exit. It is
// only safe to call from the normal end-of-stream path, i.e. once Drain has
// already returned a non-nil error - at that point req.Recv has already
// returned and the goroutine is exiting or already gone, so the wait is
// immediate.
//
// Do not call Stop from an early-exit path (e.g. a write error while more
// blocks may still be in flight): req.Recv only unblocks on data, client
// half-close, or the RPC's own context ending, and this ctx (a child of
// h.req.Context(), the RPC's own context) has no way to make that happen
// synchronously - so waiting here could block the handler's return
// indefinitely. Use Cancel instead: it only signals, so the handler returns
// right away, and the eventual cleanup is guaranteed anyway - once Put
// returns, the grpc-go runtime cancels h.req.Context() itself as part of
// ending the RPC, which is what actually unblocks the pending req.Recv and
// lets the abandoned goroutine exit on its own. Nothing else touches this
// blockReceiver's channel or req.Recv in the meantime, so a detached
// goroutine is safe.
func (r *blockReceiver) Stop() {
	r.cancel()
	<-r.done
}

func (r *blockReceiver) run(ctx context.Context, req grpc.WAL_PutServer) {
	defer close(r.ch)
	defer close(r.done)

	for {
		request, err := req.Recv()
		if err != nil {
			r.err = err

			return
		}

		select {
		case r.ch <- request:
		case <-ctx.Done():
			r.err = ctx.Err()

			return
		}
	}
}

func (r *blockReceiver) closedErr() error {
	if r.err != nil {
		return r.err
	}

	return io.EOF
}
