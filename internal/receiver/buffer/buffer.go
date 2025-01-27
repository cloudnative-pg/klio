package buffer

import (
	"bytes"
	"fmt"
	"log/slog"

	"github.com/cloudnative-pg/machinery/pkg/types"
)

// UnexpectedWalDataOffsetError is the error returned when
// the WAL data offset is not the expected one.
type UnexpectedWalDataOffsetError struct {
	offset   uint64
	expected int
}

func (e *UnexpectedWalDataOffsetError) Error() string {
	return fmt.Sprintf("Unexpected WAL data offset: %08x, expected: %08x", e.offset, e.expected)
}

// Flusher is the type of functions that are called
// to write a WAL file.
type Flusher func(walName string, data []byte) error

// Data is the implementation of the WAL buffer.
type Data struct {
	buffer         bytes.Buffer
	segmentSize    uint64
	tli            int
	currentWALFile string
	flusher        Flusher
	logger         *slog.Logger
}

// New creates a new WAL buffer.
func New(logger *slog.Logger, tli int, walSegmentSize uint64, flusher Flusher) *Data {
	return &Data{
		segmentSize: walSegmentSize,
		buffer:      *bytes.NewBuffer(make([]byte, 0, walSegmentSize)),
		tli:         tli,
		flusher:     flusher,
		logger:      logger,
	}
}

// UnopenedFileForWALError is the error returned when a WAL
// record is received without a WAL file open.
type UnopenedFileForWALError struct {
	offset uint64
}

func (e *UnopenedFileForWALError) Error() string {
	return fmt.Sprintf("received write-ahead log record for offset %v with no file open", e.offset)
}

// ProcessWALData processes a WAL message from PG
//
//nolint:cyclop
func (wal *Data) ProcessWALData(data []byte, startWAL types.LSN) error {
	// This implementation is largely based on src/bin/pg_basebackup/receivelog.c
	// [ProcessXLogDataMsg]

	//nolint:lll
	// See: https://github.com/postgres/postgres/blob/00f4c2959d631c7851da21a512885d1deab28649/src/bin/pg_basebackup/receivelog.c#L1039

	wal.logger.Debug("Process WAL Data", "lenData", len(data), "startWAL", startWAL)

	blockpos, err := startWAL.Parse()
	if err != nil {
		return fmt.Errorf("while parsing WAL data start (pos): %w", err)
	}

	xlogoff := blockpos % wal.segmentSize

	if !wal.hasWALFileOpened() {
		if xlogoff != 0 {
			// No file open yet
			return &UnopenedFileForWALError{offset: xlogoff}
		}
	} else {
		// More data in existing segment
		//nolint:gosec
		if uint64(wal.buffer.Len()) != xlogoff {
			return &UnexpectedWalDataOffsetError{offset: xlogoff, expected: wal.buffer.Len()}
		}
	}

	bytesLeft := uint64(len(data))
	bytesWritten := uint64(0)
	for bytesLeft > 0 {
		var bytesToWrite uint64

		// If crossing a WAL boundary, only write up until we reach wal
		// segment size.
		if xlogoff+bytesLeft > wal.segmentSize {
			bytesToWrite = wal.segmentSize - xlogoff
		} else {
			bytesToWrite = bytesLeft
		}

		if !wal.hasWALFileOpened() {
			if err := wal.openWALFile(blockpos); err != nil {
				return err
			}
		}

		_, err := wal.buffer.Write(data[bytesWritten : bytesWritten+bytesToWrite])
		if err != nil {
			return fmt.Errorf("while writing to WAL buffer: %w", err)
		}

		bytesWritten += bytesToWrite
		bytesLeft -= bytesToWrite
		blockpos += bytesToWrite
		xlogoff += bytesToWrite

		// Did we reach the end of a WAL segment?
		//nolint:gosec
		if uint64(wal.buffer.Len()) == wal.segmentSize {
			err := wal.closeWALFile()
			if err != nil {
				return fmt.Errorf("while flushing WAL buffer: %w", err)
			}

			xlogoff = 0
		}
	}

	return nil
}

func (wal *Data) hasWALFileOpened() bool {
	return wal.currentWALFile != ""
}

func (wal *Data) openWALFile(blockpos uint64) error {
	var err error

	wal.currentWALFile, err = types.Int64ToLSN(blockpos).WALFileName(wal.tli, wal.segmentSize)
	if err != nil {
		return fmt.Errorf("while creating WAL file name (pos %v): %w", blockpos, err)
	}
	wal.buffer.Reset()

	wal.logger.Debug("Opening WAL File", "walFileName", wal.currentWALFile)

	return nil
}

func (wal *Data) closeWALFile() error {
	wal.logger.Debug("Closing WAL File", "walFileName", wal.currentWALFile)
	if err := wal.flusher(wal.currentWALFile, wal.buffer.Bytes()); err != nil {
		return err
	}

	wal.currentWALFile = ""
	wal.buffer.Reset()

	return nil
}
