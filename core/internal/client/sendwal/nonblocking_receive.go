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

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"
)

// ErrMessageReceiverClosed is a fallback returned when the background
// receive loop of a messageReceiver has exited without recording an error,
// which should not normally happen.
var ErrMessageReceiverClosed = errors.New("message receiver closed")

// cloneMessage returns a copy of msg that is safe to keep after the next
// call to conn.ReceiveMessage.
//
// pgproto3's Frontend reads messages into a shared, reused buffer: the
// byte slices it hands back (e.g. CopyData.Data) are only valid until the
// next Receive call. The background loop in messageReceiver queues several
// messages before any of them are processed, so without copying here, an
// earlier queued message's payload gets silently overwritten by a later
// read of the same buffer.
//
//nolint:ireturn
func cloneMessage(msg pgproto3.BackendMessage) pgproto3.BackendMessage {
	if cd, ok := msg.(*pgproto3.CopyData); ok {
		data := make([]byte, len(cd.Data))
		copy(data, cd.Data)

		return &pgproto3.CopyData{Data: data}
	}

	return msg
}

// messageReceiver decouples receiving PostgreSQL protocol messages from the
// caller's polling cadence. A background goroutine blocks on
// conn.ReceiveMessage in a tight loop and publishes every message on a
// channel, so that draining messages already available on the wire never
// needs an artificial read deadline: it is answered instantly by however
// many items are already queued in the channel.
//
// Stop must be called before conn is read from by anything else - e.g.
// before pglogrepl.SendStandbyCopyDone, which reads directly off the same
// underlying frontend. Without it, the background goroutine keeps calling
// conn.ReceiveMessage after the caller has moved on, racing whoever reads
// from conn next for messages meant for them.
type messageReceiver struct {
	ch     <-chan pgproto3.BackendMessage
	cancel context.CancelFunc
	done   chan struct{}

	// err is set by the background goroutine, once, before it closes ch.
	// The Go memory model guarantees that a receive observing ch as closed
	// happens after that close, so reading err after such a receive is safe
	// without extra synchronization: only one goroutine ever writes it, and
	// only after all sends are done.
	err error
}

// newMessageReceiver starts the background receive loop for conn. The loop,
// and the returned receiver, are only valid until ctx is done or Stop is
// called, whichever happens first; after either, ReceiveAvailable starts
// failing.
func newMessageReceiver(ctx context.Context, conn *pgconn.PgConn) *messageReceiver {
	ctx, cancel := context.WithCancel(ctx)
	ch := make(chan pgproto3.BackendMessage, 500)
	receiver := &messageReceiver{ch: ch, cancel: cancel, done: make(chan struct{})}

	go func() {
		defer close(ch)
		defer close(receiver.done)

		for {
			msg, err := conn.ReceiveMessage(ctx)
			if err != nil {
				receiver.err = err

				return
			}

			select {
			case ch <- cloneMessage(msg):
			case <-ctx.Done():
				receiver.err = ctx.Err()

				return
			}
		}
	}()

	return receiver
}

// Stop cancels the background receive loop and waits for it to exit. It is
// safe to call more than once. Once Stop returns, the background goroutine
// is no longer calling conn.ReceiveMessage, so conn is safe to read from
// directly.
func (r *messageReceiver) Stop() {
	r.cancel()
	<-r.done
}

// ReceiveAvailable blocks until at least one message is available, then
// drains every message that was already queued without blocking again -
// so a burst of messages already on the wire is handed to the caller
// together, letting it process them (and flush the result) as one batch
// instead of one at a time.
func (r *messageReceiver) ReceiveAvailable(ctx context.Context) ([]pgproto3.BackendMessage, error) {
	select {
	case msg, ok := <-r.ch:
		if !ok {
			return nil, r.closedErr()
		}

		messages := make([]pgproto3.BackendMessage, 0, 1)
		messages = append(messages, msg)

		for {
			select {
			case msg, ok := <-r.ch:
				if !ok {
					return messages, nil
				}

				messages = append(messages, msg)
			default:
				return messages, nil
			}
		}

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// closedErr returns the error recorded by the background goroutine once it
// has closed the channel, falling back to ErrMessageReceiverClosed in the
// (unexpected) case none was recorded.
func (r *messageReceiver) closedErr() error {
	if r.err != nil {
		return r.err
	}

	return ErrMessageReceiverClosed
}
