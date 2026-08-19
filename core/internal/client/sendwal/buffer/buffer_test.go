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

package buffer

import (
	"context"
	"testing"

	"github.com/cloudnative-pg/machinery/pkg/types"
)

// fakeHandler is a Handler whose durable offset can be controlled by the test,
// letting it stand in for a Klio server that acknowledges durability lazily.
type fakeHandler struct {
	opened  bool
	written uint64
	synced  uint64
}

func (f *fakeHandler) HasWALFileOpened() bool { return f.opened }

func (f *fakeHandler) OpenWAL(_ context.Context, _ uint64) error {
	f.opened = true
	f.written = 0
	f.synced = 0

	return nil
}

func (f *fakeHandler) CloseWAL(_ context.Context) error {
	f.opened = false
	return nil
}

func (f *fakeHandler) CurrentOffset() (uint64, error) { return f.written, nil }

func (f *fakeHandler) Write(_ context.Context, p []byte) (int, error) {
	f.written += uint64(len(p))
	return len(p), nil
}

func (f *fakeHandler) SyncedOffset() (uint64, error) { return f.synced, nil }

const testSegmentSize = 16 * 1024 * 1024

// TestFlushLSNTracksWriteWhenDurableAckDisabled verifies that, in the default
// mode, the flush position advances to the written position as soon as data is
// handed to the send buffer.
func TestFlushLSNTracksWriteWhenDurableAckDisabled(t *testing.T) {
	ctx := context.Background()
	handler := &fakeHandler{}
	data := New(1, testSegmentSize, handler, 64*1024, false)

	if err := data.ProcessWALData(ctx, make([]byte, 1000), types.LSN("0/0")); err != nil {
		t.Fatalf("processing WAL data: %v", err)
	}
	if err := data.Flush(ctx); err != nil {
		t.Fatalf("flushing: %v", err)
	}

	if data.WriteLSN() != 1000 {
		t.Fatalf("expected write LSN 1000, got %d", data.WriteLSN())
	}
	if data.FlushLSN() != 1000 {
		t.Fatalf("expected flush LSN to track write LSN (1000), got %d", data.FlushLSN())
	}
}

// TestFlushLSNGatedOnDurableAck verifies that, when durable acknowledgements are
// required, the flush position only advances up to the offset the handler
// reports as durable, never ahead of it.
func TestFlushLSNGatedOnDurableAck(t *testing.T) {
	ctx := context.Background()
	handler := &fakeHandler{}
	data := New(1, testSegmentSize, handler, 64*1024, true)

	if err := data.ProcessWALData(ctx, make([]byte, 1000), types.LSN("0/0")); err != nil {
		t.Fatalf("processing WAL data: %v", err)
	}

	// Nothing acknowledged yet: the write position moves, the flush position
	// must not.
	if err := data.Flush(ctx); err != nil {
		t.Fatalf("flushing: %v", err)
	}
	if data.WriteLSN() != 1000 {
		t.Fatalf("expected write LSN 1000, got %d", data.WriteLSN())
	}
	if data.FlushLSN() != 0 {
		t.Fatalf("expected flush LSN 0 before any ack, got %d", data.FlushLSN())
	}

	// A partial acknowledgement advances the flush position up to the durable
	// offset only.
	handler.synced = 600
	if err := data.Flush(ctx); err != nil {
		t.Fatalf("flushing: %v", err)
	}
	if data.FlushLSN() != 600 {
		t.Fatalf("expected flush LSN 600 after partial ack, got %d", data.FlushLSN())
	}

	// Full acknowledgement lets the flush position reach the write position.
	handler.synced = 1000
	if err := data.Flush(ctx); err != nil {
		t.Fatalf("flushing: %v", err)
	}
	if data.FlushLSN() != 1000 {
		t.Fatalf("expected flush LSN 1000 after full ack, got %d", data.FlushLSN())
	}
}

// TestFlushLSNNeverRegresses verifies that a smaller durable offset reported
// afterwards cannot move the flush position backwards.
func TestFlushLSNNeverRegresses(t *testing.T) {
	ctx := context.Background()
	handler := &fakeHandler{}
	data := New(1, testSegmentSize, handler, 64*1024, true)

	if err := data.ProcessWALData(ctx, make([]byte, 1000), types.LSN("0/0")); err != nil {
		t.Fatalf("processing WAL data: %v", err)
	}

	handler.synced = 800
	if err := data.Flush(ctx); err != nil {
		t.Fatalf("flushing: %v", err)
	}
	if data.FlushLSN() != 800 {
		t.Fatalf("expected flush LSN 800, got %d", data.FlushLSN())
	}

	handler.synced = 500
	if err := data.Flush(ctx); err != nil {
		t.Fatalf("flushing: %v", err)
	}
	if data.FlushLSN() != 800 {
		t.Fatalf("expected flush LSN to stay at 800, got %d", data.FlushLSN())
	}
}

// TestFlushLSNPinnedOnSegmentBoundary verifies that completing a WAL segment
// advances the flush position to the segment boundary, because closing the file
// drains and verifies every outstanding acknowledgement.
func TestFlushLSNPinnedOnSegmentBoundary(t *testing.T) {
	ctx := context.Background()
	handler := &fakeHandler{}
	const smallSegment = 2048
	data := New(1, smallSegment, handler, 64*1024, true)

	// Writing exactly one segment crosses the boundary and closes the file.
	if err := data.ProcessWALData(ctx, make([]byte, smallSegment), types.LSN("0/0")); err != nil {
		t.Fatalf("processing WAL data: %v", err)
	}

	if data.WriteLSN() != smallSegment {
		t.Fatalf("expected write LSN %d, got %d", smallSegment, data.WriteLSN())
	}
	// Even though no ack was recorded, the closed segment is durable.
	if data.FlushLSN() != smallSegment {
		t.Fatalf("expected flush LSN pinned to boundary %d, got %d", smallSegment, data.FlushLSN())
	}
}
