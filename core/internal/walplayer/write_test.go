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

package walplayer

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ccoveille/go-safecast/v2"
)

func TestWALWriterWriteWALCreatesFileWithCorrectSize(t *testing.T) {
	dir := t.TempDir()
	fileName := filepath.Join(dir, "testwal")
	writer := &WALWriter{
		reader:      NewLoopReader([]byte{0xAA, 0xBB, 0xCC}),
		segmentSize: 1024,
		position:    0,
	}
	err := writer.writeWAL(t.Context(), fileName)
	if err != nil {
		t.Fatalf("writeWAL failed: %v", err)
	}
	info, err := os.Stat(fileName)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}

	segmentSize, err := safecast.Convert[int64](writer.segmentSize)
	if err != nil {
		t.Fatalf("while converting segment size to int64: %v", err)
	}

	if info.Size() != segmentSize {
		t.Errorf("expected file size %d, got %d", writer.segmentSize, info.Size())
	}
}

func TestWALWriterToDirectoryCreatesMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	writer := &WALWriter{
		reader:      NewLoopReader([]byte{0x01, 0x02}),
		segmentSize: 256,
		position:    0,
	}
	segments := 3
	err := writer.ToDirectory(t.Context(), dir, segments)
	if err != nil {
		t.Fatalf("ToDirectory failed: %v", err)
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}
	if len(files) != segments {
		t.Errorf("expected %d files, got %d", segments, len(files))
	}
}

func TestWALWriterWriteWALErrorOnInvalidPath(t *testing.T) {
	writer := &WALWriter{
		reader:      NewLoopReader([]byte{0x01}),
		segmentSize: 10,
		position:    0,
	}
	err := writer.writeWAL(t.Context(), "/invalid/path/shouldfail")
	if err == nil {
		t.Error("expected error for invalid path, got nil")
	}
}

func TestWALWriterWriteWALErrorOnShortReader(t *testing.T) {
	// io.LimitReader returns EOF before segmentSize is reached
	writer := &WALWriter{
		reader:      io.LimitReader(NewLoopReader([]byte{0x01}), 5),
		segmentSize: 10,
		position:    0,
	}
	dir := t.TempDir()
	fileName := filepath.Join(dir, "short")
	err := writer.writeWAL(t.Context(), fileName)
	if err == nil {
		t.Error("expected error due to short reader, got nil")
	}
}
