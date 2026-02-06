package walplayer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ccoveille/go-safecast/v2"
	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/grpcclient"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// WALUploadTask represent a WAL file to be sent.
type WALUploadTask struct {
	// The wal name
	WALFullPath string
}

// WALUploadReport contains the statistics that were collected
// when uploading a WAL.
type WALUploadReport struct {
	// The full path including the wal name
	WALFullPath string `json:"walFullPath"`

	// When the process started
	StartTime time.Time `json:"startTime"`

	// When the process stopped
	EndTime time.Time `json:"endTime"`

	// The difference between the end and the start time
	ElapsedTime time.Duration `json:"elapsedTime"`

	// If the WAL upload process failed, the error,
	// otherwise empty.
	Error string `json:"error"`
}

// runManager reads the WAL files from a directory and send them in a work queue.
// The queue is closed after the WAL files are finished.
func runManager(ctx context.Context, dirname string, queue chan<- WALUploadTask) {
	contextLogger := log.FromContext(ctx)
	defer close(queue)

	contextLogger.Info("Starting manager goroutine", "dirname", dirname)
	entries, err := os.ReadDir(dirname)
	if err != nil {
		contextLogger.Error(err, "While reading WAL files")
		return
	}

	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return

		default:
			queue <- WALUploadTask{
				WALFullPath: path.Join(dirname, entry.Name()),
			}
		}
	}
}

// runCollector starts a goroutine that collect the results of the worker goroutines.
func runCollector(ctx context.Context, resultsChan <-chan WALUploadReport) []WALUploadReport {
	contextLogger := log.FromContext(ctx)
	results := make([]WALUploadReport, 0)

	contextLogger.Info("Starting collector goroutine")
	defer contextLogger.Info("Stopping collector goroutine")

loop:
	for {
		select {
		case <-ctx.Done():
			break loop

		case t, ok := <-resultsChan:
			if !ok {
				break loop
			}

			t.ElapsedTime = t.EndTime.Sub(t.StartTime)
			results = append(results, t)
		}
	}

	return results
}

// Player is the implementation of the WAL player feature.
type Player struct {
	// The number of concurrent jobs.
	Workers int

	// The directory where to look for WALs.
	DirName string

	// The size of the block that is sent to the Klio server.
	BlockSize int

	// The configuration to be used to connect to the Klio server.
	ClientConfig *config.ClientConfig
}

// NewPlayer creates a new Player instance with the given parameters.
func NewPlayer(workers int, targetDir string, blockSize int, clientConfig *config.ClientConfig) *Player {
	return &Player{
		Workers:      workers,
		DirName:      targetDir,
		BlockSize:    blockSize,
		ClientConfig: clientConfig,
	}
}

// Play sends a directory of WAL files to a Klio server.
func (p *Player) Play(ctx context.Context) []WALUploadReport {
	queue := make(chan WALUploadTask)
	resultChannel := make(chan WALUploadReport)
	var results []WALUploadReport

	var wg sync.WaitGroup
	wg.Go(func() {
		runManager(ctx, p.DirName, queue)
	})

	wg.Go(func() {
		results = runCollector(ctx, resultChannel)
	})

	var wgWorkers sync.WaitGroup
	for range p.Workers {
		wgWorkers.Go(func() {
			p.runWorker(ctx, queue, resultChannel)
		})
	}
	wgWorkers.Wait()
	close(resultChannel)

	wg.Wait()

	return results
}

// runWorker starts a worker goroutine that, until the end of the queue, grabs
// tasks and execute them. Results are sent to the passed channel.
func (p *Player) runWorker(
	ctx context.Context,
	queue <-chan WALUploadTask,
	collector chan<- WALUploadReport,
) {
	contextLogger := log.FromContext(ctx)

	client, err := grpcclient.Connect(p.ClientConfig, p.ClientConfig.Wal.Address)
	if err != nil {
		contextLogger.Error(err, "While connecting to Klio")
		return
	}

	contextLogger.Info("Starting worker goroutine")
	defer contextLogger.Info("Stopping worker goroutine")

	for {
		select {
		case <-ctx.Done():
			return

		case t, ok := <-queue:
			if !ok {
				return
			}

			result := p.sendWAL(ctx, client, t.WALFullPath)
			collector <- *result
		}
	}
}

//nolint:cyclop
func (p *Player) sendWAL(ctx context.Context, c *grpcclient.Connection, fileName string) *WALUploadReport {
	contextLogger := log.FromContext(ctx)

	result := &WALUploadReport{
		WALFullPath: fileName,
		StartTime:   time.Now(),
	}
	defer func() {
		result.EndTime = time.Now()
		result.ElapsedTime = result.EndTime.Sub(result.StartTime)

		contextLogger.Info("WAL file sent to the server",
			"fileName", fileName, "elapsedTime", result.ElapsedTime)
	}()

	var f ReaderSizerCloser
	var err error

	if strings.HasSuffix(fileName, ".gz") {
		f, err = NewGZIPReaderSizer(fileName)
		fileName = strings.TrimSuffix(fileName, filepath.Ext(fileName))
	} else {
		f, err = NewUncompressedFileReader(fileName)
	}
	if err != nil {
		result.Error = err.Error()
		return result
	}

	defer func() {
		if err := f.Close(); err != nil {
			contextLogger.Error(err, "Got error while closing file, skipping.")
		}
	}()

	size, err := safecast.Convert[uint64](f.Size())
	if err != nil {
		result.Error = fmt.Sprintf("while converting file size: %v", err)
		return result
	}

	stream, err := c.StoreWALStreaming(ctx, path.Base(fileName), size, false)
	if err != nil {
		result.Error = fmt.Sprintf("while starting WAL file streaming: %v", err)
		return result
	}

	block := make([]byte, p.BlockSize)
l:
	for {
		n, err := io.ReadFull(f, block)
		switch {
		case errors.Is(err, io.EOF):
			break l

		case err != nil && !errors.Is(err, io.ErrUnexpectedEOF):
			result.Error = fmt.Sprintf("while reading WAL file: %v", err)
			break l
		}

		if err := stream.SendBlock(ctx, block[:n]); err != nil {
			result.Error = fmt.Sprintf("while sending WAL file block: %v", err)
			break l
		}
	}

	if err := stream.Close(ctx); err != nil {
		result.Error = fmt.Sprintf("while closing WAL stream: %v", err)
		return result
	}

	return result
}
