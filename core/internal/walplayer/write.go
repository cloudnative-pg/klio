package walplayer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/ccoveille/go-safecast/v2"
	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/cloudnative-pg/machinery/pkg/types"
)

// WALWriter write random WAL files.
type WALWriter struct {
	reader      io.Reader
	segmentSize uint64
	position    uint64
}

// NewWALWriter creates a WALWriter.
func NewWALWriter(sizeMB int) (*WALWriter, error) {
	uint64SizeMB, err := safecast.Convert[uint64](sizeMB)
	if err != nil {
		return nil, fmt.Errorf("while converting size to uint64: %w", err)
	}

	return &WALWriter{
		reader:      NewLoopReader(buffer),
		segmentSize: uint64SizeMB * 1024 * 1024,
		position:    0,
	}, nil
}

// ToDirectory writes a set of WAL files into the target directory, each one
// having the specified size.
func (w *WALWriter) ToDirectory(ctx context.Context, dirname string, segmentsCount int) error {
	for range segmentsCount {
		name, err := types.Int64ToLSN(w.position).WALFileName(1, w.segmentSize)
		if err != nil {
			return fmt.Errorf("while formatting WAL file name (position %q): %w", w.position, err)
		}

		if err := w.writeWAL(ctx, path.Join(dirname, name)); err != nil {
			return err
		}

		w.position += w.segmentSize
	}

	return nil
}

// writeWAL writes a WAL file with a certain name and a defined size.
func (w *WALWriter) writeWAL(ctx context.Context, fileName string) error {
	contextLogger := log.FromContext(ctx)
	contextLogger.Info("Writing WAL file", "fileName", fileName)

	f, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) //nolint:gosec
	if err != nil {
		return fmt.Errorf("error opening file %q: %w", fileName, err)
	}

	defer func() {
		err := f.Close()
		if err != nil {
			contextLogger.Error(err, "error closing file", "file", fileName)
		}
	}()

	int64SegmentSize, err := safecast.Convert[int64](w.segmentSize)
	if err != nil {
		return fmt.Errorf("while converting segment size to int64: %w", err)
	}

	if _, err := io.CopyN(f, w.reader, int64SegmentSize); err != nil {
		return fmt.Errorf("error writing to file %q: %w", fileName, err)
	}

	return nil
}
