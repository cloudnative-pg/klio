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

package grpcclient

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSegmentSize = uint64(16 * 1024 * 1024)

func TestNewKlioClientHandler(t *testing.T) {
	conn := newTestConnection(t.Context(), t, "handler-cluster")

	// GIVEN a set of streaming parameters
	handler := NewKlioClientHandler(1, testSegmentSize, conn, true)

	// THEN the handler is built with those parameters and starts with no
	// WAL file open
	assert.Equal(t, 1, handler.tli)
	assert.Equal(t, testSegmentSize, handler.segmentSize)
	assert.True(t, handler.sendToTier2)
	assert.False(t, handler.HasWALFileOpened())
}

func TestNewKlioClientHandlerFactory(t *testing.T) {
	conn := newTestConnection(t.Context(), t, "factory-cluster")

	// GIVEN a factory bound to a connection and a tier2 flag
	factory := NewKlioClientHandlerFactory(conn, false)

	// WHEN it is invoked with a timeline and segment size, as sendwal.Process
	// does on every (re)start of replication
	handler := factory(2, testSegmentSize)

	// THEN it returns a working buffer.Handler for that timeline/segment size
	require.NotNil(t, handler)
	assert.False(t, handler.HasWALFileOpened())

	concreteHandler, ok := handler.(*KlioClientStreamingHandler)
	require.True(t, ok, "factory should build a *KlioClientStreamingHandler")
	assert.Equal(t, 2, concreteHandler.tli)
	assert.Equal(t, testSegmentSize, concreteHandler.segmentSize)
}

func TestKlioClientStreamingHandlerLifecycle(t *testing.T) {
	ctx := t.Context()
	conn := newTestConnection(ctx, t, "lifecycle-cluster")

	// The segment size is set to match the payload exactly, so the
	// destination treats the WAL file as complete (rather than a
	// ".partial" segment) and stores it under its exact name, without
	// zero-padding it up to a real 16MiB segment.
	payload := []byte("wal-block-content")
	segmentSize := uint64(len(payload))

	factory := NewKlioClientHandlerFactory(conn, false)
	handler := factory(1, segmentSize)
	concreteHandler, ok := handler.(*KlioClientStreamingHandler)
	require.True(t, ok, "factory should build a *KlioClientStreamingHandler")

	// GIVEN a freshly created handler
	require.False(t, handler.HasWALFileOpened())

	// WHEN a WAL file is opened at the start of a segment
	require.NoError(t, handler.OpenWAL(ctx, 0))

	// THEN the handler reports a WAL file as open, with a zero offset
	assert.True(t, handler.HasWALFileOpened())

	// Captured now: CloseWAL below resets currentWALFile to "".
	walName := concreteHandler.currentWALFile

	offset, err := handler.CurrentOffset()
	require.NoError(t, err)
	assert.Equal(t, uint64(0), offset)

	// WHEN the whole segment is written in a single block
	n, err := handler.Write(ctx, payload)
	require.NoError(t, err)

	// THEN the write is fully accounted for and the offset advances
	assert.Equal(t, len(payload), n)

	offset, err = handler.CurrentOffset()
	require.NoError(t, err)
	assert.Equal(t, uint64(len(payload)), offset)

	// WHEN the WAL file is closed
	require.NoError(t, handler.CloseWAL(ctx))

	// THEN it is reported as no longer open, and what was written round-trips
	// back unchanged when downloaded from the destination
	assert.False(t, handler.HasWALFileOpened())

	var downloaded bytes.Buffer
	require.NoError(t, conn.GetWALStreaming(ctx, walName, &downloaded))
	assert.Equal(t, payload, downloaded.Bytes())
}
