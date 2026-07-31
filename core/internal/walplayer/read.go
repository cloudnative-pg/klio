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
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// Sizer interface is implemented by streams knowing their exact size.
type Sizer interface {
	// Size gets the file size in bytes
	Size() int64
}

// ReaderSizerCloser is a combination of Reader, Closer and Sizer.
type ReaderSizerCloser interface {
	io.Reader
	io.Closer
	Sizer
}

// UncompressedWALReader implements a file reader and sizer for uncompressed files.
type UncompressedWALReader struct {
	*os.File

	size int64
}

// NewUncompressedFileReader creates an uncompressed file reader and sizer for
// files.
func NewUncompressedFileReader(fileName string) (*UncompressedWALReader, error) {
	fileInfo, err := os.Stat(fileName)
	if err != nil {
		return nil, fmt.Errorf("while getting file size: %w", err)
	}

	f, err := os.Open(fileName) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("while opening file: %w", err)
	}

	return &UncompressedWALReader{
		File: f,
		size: fileInfo.Size(),
	}, nil
}

// Size gets the uncompressed size of the WAL size.
func (r *UncompressedWALReader) Size() int64 {
	return r.size
}

// GZIPReaderSizer implements a Reader and Sizer for files compressed
// using the GZIP strategy.
type GZIPReaderSizer struct {
	*gzip.Reader

	size     int64
	fileName string
}

// NewGZIPReaderSizer creates a new GZIP files reader and sizer.
func NewGZIPReaderSizer(fileName string) (*GZIPReaderSizer, error) {
	f, err := os.Open(fileName) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("while opening file: %w", err)
	}

	// To get the uncompressed size, we need to read the last four
	// bytes.
	if _, err := f.Seek(-4, io.SeekEnd); err != nil {
		return nil, fmt.Errorf("while seeking file (going to the end): %w", err)
	}

	var bsize [4]byte
	if _, err := io.ReadFull(f, bsize[:]); err != nil {
		return nil, fmt.Errorf("while reading size of uncompressed file: %w", err)
	}

	size := binary.LittleEndian.Uint32(bsize[:])

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("while seeking file (going to the start): %w", err)
	}

	reader, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("while reading GZIP stream: %w", err)
	}

	return &GZIPReaderSizer{
		Reader:   reader,
		size:     int64(size),
		fileName: fileName,
	}, nil
}

// Size implements the Sizer interface.
func (r *GZIPReaderSizer) Size() int64 {
	return r.size
}
