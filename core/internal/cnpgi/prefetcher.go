package cnpgi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/sourcegraph/conc/pool"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/grpcclient"
)

const (
	// defaultPrefetchCount is the number of WAL files to prefetch ahead.
	defaultPrefetchCount = 2

	// defaultMaxConcurrentDownloads is the default maximum number of concurrent WAL downloads.
	defaultMaxConcurrentDownloads = 4

	// defaultDownloadTimeout is the maximum time allowed for a single WAL download.
	defaultDownloadTimeout = 10 * time.Minute

	// maxWALBytesPerLog is 4GB, used to calculate segments per log.
	maxWALBytesPerLog = 0x100000000

	// spoolDirName is the name of the spool directory under pg_wal.
	spoolDirName = ".klio-spool"
)

// walState represents the state of a WAL file in the prefetcher.
type walState int

const (
	walStateDownloading walState = iota
	walStateReady
	walStateFailed
)

// walEntry represents a WAL file being tracked by the prefetcher.
type walEntry struct {
	state      walState
	spoolPath  string
	err        error
	done       chan struct{}
	isPrefetch bool // true if this was a speculative prefetch, false if PG requested it
}

// walPrefetcher manages prefetching of WAL files for faster recovery.
type walPrefetcher struct {
	mu              sync.Mutex
	entries         map[string]*walEntry
	spoolDir        string
	connection      *grpcclient.Connection
	prefetchCount   int
	downloadTimeout time.Duration

	// downloadPool limits concurrent WAL downloads to avoid spawning unbounded
	// goroutines. Uses a ContextPool so we can cancel pending downloads on shutdown.
	downloadPool   *pool.ContextPool
	cancelDownload context.CancelFunc

	// Learned from first successful download.
	segmentSize    uint64
	segmentsPerLog uint64
}

// newWALPrefetcher creates a new WAL prefetcher.
func newWALPrefetcher(
	spoolDir string,
	connection *grpcclient.Connection,
	prefetchCount int,
	maxConcurrentDownloads int,
) *walPrefetcher {
	if prefetchCount <= 0 {
		prefetchCount = defaultPrefetchCount
	}
	if maxConcurrentDownloads <= 0 {
		maxConcurrentDownloads = defaultMaxConcurrentDownloads
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &walPrefetcher{
		entries:         make(map[string]*walEntry),
		spoolDir:        spoolDir,
		connection:      connection,
		prefetchCount:   prefetchCount,
		downloadTimeout: defaultDownloadTimeout,
		downloadPool:    pool.New().WithContext(ctx).WithMaxGoroutines(maxConcurrentDownloads),
		cancelDownload:  cancel,
	}
}

// Request retrieves a WAL file, using the prefetch cache if available.
// It also triggers prefetching of subsequent WAL files.
func (p *walPrefetcher) Request(ctx context.Context, walName, targetPath string) error {
	contextLogger := log.FromContext(ctx).WithValues("walName", walName)

	// Try to get complete WAL from cache or download.
	err := p.getCompleteWAL(ctx, walName, targetPath)
	if err == nil {
		// Success - trigger prefetch of next N complete WALs.
		p.mu.Lock()
		canPrefetch := p.segmentSize > 0
		p.mu.Unlock()

		if canPrefetch {
			p.triggerPrefetch(walName)
		}

		return nil
	}

	if !errors.Is(err, errWALNotFound) {
		return err
	}

	// Only a bare WAL segment can have a .partial variant, so don't fabricate a
	// nonsensical "<name>.partial" request for a history or backup-label file:
	// report it as missing and let the caller move on.
	if !canHavePartial(walName) {
		return err
	}

	// Complete WAL not found - try partial (direct to target, no cache).
	contextLogger.Debug("Complete WAL not found, trying partial")

	return p.getPartialWAL(ctx, walName, targetPath)
}

// canHavePartial reports whether walName could have a .partial variant. Only a
// bare WAL segment can; a history or backup-label file (anything carrying an
// extension) cannot. This matches the standalone get-wal command, which also
// only falls back to a .partial for extension-less names.
func canHavePartial(walName string) bool {
	return filepath.Ext(walName) == ""
}

// Close shuts down the prefetcher, canceling pending downloads and waiting
// for in-flight downloads to complete.
func (p *walPrefetcher) Close() error {
	p.cancelDownload()

	return p.downloadPool.Wait()
}

// getCompleteWAL retrieves a complete WAL file from cache or downloads it.
//
//nolint:cyclop // complexity is slightly over limit but refactoring would hurt readability
func (p *walPrefetcher) getCompleteWAL(ctx context.Context, walName, targetPath string) error {
	contextLogger := log.FromContext(ctx).WithValues("walName", walName)

	p.mu.Lock()
	entry, exists := p.entries[walName]

	// If a previous attempt failed, discard it and retry. The error could have
	// been transient (connection issue, timeout) or the file might exist now
	// (was not found before but primary has since created it).
	if exists && entry.state == walStateFailed {
		delete(p.entries, walName)
		exists = false
	}

	// A cache hit is when we have a prefetched entry that's already ready.
	prefetchHit := exists && entry.isPrefetch && entry.state == walStateReady

	if !exists {
		// Not prefetched - start download now (direct request from PG).
		entry = p.startDirectDownloadLocked(ctx, walName)
	}
	p.mu.Unlock()

	if prefetchHit {
		contextLogger.Info("WAL prefetch cache hit")
	}

	// Wait for download to complete.
	select {
	case <-entry.done:
	case <-ctx.Done():
		return ctx.Err()
	}

	if entry.err != nil {
		return entry.err
	}

	// Rename from spool to target (atomic on same filesystem).
	if err := os.Rename(entry.spoolPath, targetPath); err != nil {
		contextLogger := log.FromContext(ctx)
		contextLogger.Info("Rename failed, falling back to direct download",
			"walName", walName,
			"spoolPath", entry.spoolPath,
			"targetPath", targetPath,
			"error", err,
		)

		// Clean up - spool state is uncertain.
		p.cleanupEntry(walName)
		_ = os.Remove(entry.spoolPath)

		// Download directly to target.
		return p.downloadDirect(ctx, walName, targetPath)
	}

	// Cleanup entry from map (file already moved).
	p.cleanupEntry(walName)

	return nil
}

// downloadWALToFile downloads a WAL file to the specified path.
// This is the core download logic used by other methods.
func (p *walPrefetcher) downloadWALToFile(ctx context.Context, walName, destPath string) error {
	output, err := os.OpenFile(destPath, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec
	if err != nil {
		return fmt.Errorf("opening file %q: %w", destPath, err)
	}

	err = p.connection.GetWALStreaming(ctx, walName, output)
	closeErr := output.Close()

	if err != nil {
		// Clean up partial file on error.
		_ = os.Remove(destPath)

		if errors.Is(err, klioclient.ErrMissingWALFile) {
			return errWALNotFound
		}

		return fmt.Errorf("downloading WAL %q: %w", walName, err)
	}

	if closeErr != nil {
		_ = os.Remove(destPath)

		return fmt.Errorf("closing file %q: %w", destPath, closeErr)
	}

	return nil
}

// downloadDirect downloads a complete WAL file directly to the target path,
// bypassing the spool. Used as a fallback when rename from spool fails.
func (p *walPrefetcher) downloadDirect(ctx context.Context, walName, targetPath string) error {
	contextLogger := log.FromContext(ctx).WithValues(
		"walName", walName,
		"targetPath", targetPath,
		"downloadType", "direct-fallback",
	)
	contextLogger.Debug("Starting direct WAL download")

	// Add timeout to prevent downloads from hanging forever.
	downloadCtx, cancel := context.WithTimeout(ctx, p.downloadTimeout)
	defer cancel()

	start := time.Now()
	err := p.downloadWALToFile(downloadCtx, walName, targetPath)
	duration := time.Since(start)

	if err != nil {
		if errors.Is(err, errWALNotFound) {
			contextLogger.Info("WAL not found", "duration", duration)
		} else {
			contextLogger.Info("WAL download failed", "duration", duration, "error", err)
		}

		return err
	}

	contextLogger.Info("WAL download completed", "duration", duration)

	return nil
}

// getPartialWAL downloads a partial WAL file directly to the target path.
// Partial files are never cached as they may still be written to. The outcome
// is logged so that recovery from a .partial segment (the last segment of an
// interrupted timeline) is visible, rather than being indistinguishable from a
// complete-WAL restore.
func (p *walPrefetcher) getPartialWAL(ctx context.Context, walName, targetPath string) error {
	partialName := walName + ".partial"
	contextLogger := log.FromContext(ctx).WithValues("walName", partialName, "downloadType", "partial")

	start := time.Now()
	err := p.downloadWALToFile(ctx, partialName, targetPath)
	duration := time.Since(start)

	switch {
	case errors.Is(err, errWALNotFound):
		contextLogger.Info("Partial WAL not found", "duration", duration)
	case err != nil:
		contextLogger.Info("Partial WAL download failed", "duration", duration, "error", err)
	default:
		contextLogger.Info("Partial WAL download completed", "duration", duration)
	}

	return err
}

// startDirectDownloadLocked starts downloading a WAL file to the spool directory
// for a direct request from PostgreSQL. Must be called with p.mu held.
// The provided context is used to cancel the download if the request is canceled.
func (p *walPrefetcher) startDirectDownloadLocked(ctx context.Context, walName string) *walEntry {
	entry := &walEntry{
		state:      walStateDownloading,
		spoolPath:  filepath.Join(p.spoolDir, walName),
		done:       make(chan struct{}),
		isPrefetch: false,
	}
	p.entries[walName] = entry

	p.downloadPool.Go(func(_ context.Context) error {
		// Use the request context with timeout for direct downloads.
		downloadCtx, cancel := context.WithTimeout(ctx, p.downloadTimeout)
		defer cancel()

		p.downloadToSpool(downloadCtx, walName, entry)

		return nil
	})

	return entry
}

// startPrefetchDownloadLocked starts downloading a WAL file to the spool directory
// as a speculative prefetch. Must be called with p.mu held.
// Prefetch downloads use the pool's internal context and are not tied to any request.
func (p *walPrefetcher) startPrefetchDownloadLocked(walName string) *walEntry {
	entry := &walEntry{
		state:      walStateDownloading,
		spoolPath:  filepath.Join(p.spoolDir, walName),
		done:       make(chan struct{}),
		isPrefetch: true,
	}
	p.entries[walName] = entry

	p.downloadPool.Go(func(poolCtx context.Context) error {
		// Use the pool context with timeout for prefetch downloads.
		downloadCtx, cancel := context.WithTimeout(poolCtx, p.downloadTimeout)
		defer cancel()

		p.downloadToSpool(downloadCtx, walName, entry)

		return nil
	})

	return entry
}

// downloadToSpool downloads a WAL file to the spool directory.
// Entry fields (state, err) are written only by this function for a given entry
// (single-writer pattern). The done channel is the synchronization point: readers
// must wait on <-entry.done before accessing entry.err.
func (p *walPrefetcher) downloadToSpool(ctx context.Context, walName string, entry *walEntry) {
	defer close(entry.done)

	downloadType := "direct"
	if entry.isPrefetch {
		downloadType = "prefetch"
	}

	contextLogger := log.FromContext(ctx).WithValues(
		"walName", walName,
		"spoolPath", entry.spoolPath,
		"downloadType", downloadType,
	)
	contextLogger.Debug("Starting WAL download")

	start := time.Now()
	err := p.downloadWALToFile(ctx, walName, entry.spoolPath)
	duration := time.Since(start)

	if err != nil {
		entry.err = err
		entry.state = walStateFailed

		if errors.Is(err, errWALNotFound) {
			contextLogger.Info("WAL not found", "duration", duration)
		} else {
			contextLogger.Info("WAL download failed", "duration", duration, "error", err)
		}

		return
	}

	// Learn segment size only from complete WAL files (not history files).
	// The lock used here will only be acquired when the lock above is released;
	// there will never be two concurrent PostgreSQL instances on different
	// timelines requesting their WALs on different timelines.
	if isCompleteWALName(walName) {
		p.learnSegmentSize(ctx, entry.spoolPath)
	}

	entry.state = walStateReady
	contextLogger.Info("WAL download completed", "duration", duration)
}

// learnSegmentSize learns the segment size from a downloaded WAL file.
func (p *walPrefetcher) learnSegmentSize(ctx context.Context, spoolPath string) {
	info, err := os.Stat(spoolPath)
	if err != nil {
		return
	}

	fileSize := info.Size()
	if fileSize <= 0 {
		return
	}

	size := uint64(fileSize)

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.segmentSize == 0 {
		p.segmentSize = size
		p.segmentsPerLog = maxWALBytesPerLog / size

		contextLogger := log.FromContext(ctx)
		contextLogger.Info("Learned WAL segment size",
			"segmentSize", size,
			"segmentsPerLog", p.segmentsPerLog,
		)
	}
}

// triggerPrefetch starts downloading the next N WAL files in the background.
func (p *walPrefetcher) triggerPrefetch(currentWAL string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.segmentsPerLog == 0 {
		return
	}

	walName := currentWAL
	for range p.prefetchCount {
		var err error
		walName, err = p.nextWALNameLocked(walName)
		if err != nil {
			return
		}

		if _, exists := p.entries[walName]; !exists {
			// Start a speculative prefetch download.
			p.startPrefetchDownloadLocked(walName)
		}
	}
}

// nextWALNameLocked calculates the next WAL file name.
// Must be called with p.mu held.
func (p *walPrefetcher) nextWALNameLocked(walName string) (string, error) {
	if len(walName) != 24 {
		return "", fmt.Errorf("invalid WAL name length: %d", len(walName))
	}

	timeline, err := strconv.ParseUint(walName[0:8], 16, 32)
	if err != nil {
		return "", fmt.Errorf("parsing timeline: %w", err)
	}

	logNum, err := strconv.ParseUint(walName[8:16], 16, 32)
	if err != nil {
		return "", fmt.Errorf("parsing log number: %w", err)
	}

	segNum, err := strconv.ParseUint(walName[16:24], 16, 32)
	if err != nil {
		return "", fmt.Errorf("parsing segment number: %w", err)
	}

	// Increment segment.
	segNum++
	if segNum >= p.segmentsPerLog {
		segNum = 0
		logNum++
	}

	return fmt.Sprintf("%08X%08X%08X", timeline, logNum, segNum), nil
}

// isCompleteWALName returns true if the name is a valid complete WAL file name
// (24 hexadecimal characters, no suffix like .partial or .history).
func isCompleteWALName(name string) bool {
	return len(name) == 24
}

// cleanupEntry removes an entry from the map.
// The spool file should already be moved/deleted by the caller.
func (p *walPrefetcher) cleanupEntry(walName string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.entries, walName)
}

// spoolSubdir generates a subdirectory name for a given walRestoreOptions.
func spoolSubdir(opts walRestoreOptions) string {
	// Use the base name of the config file for readability.
	name := filepath.Base(opts.configFile)

	// Hash for uniqueness in case of collisions.
	h := sha256.Sum256([]byte(opts.configFile))
	hash := hex.EncodeToString(h[:4])

	return fmt.Sprintf("%s-%s-%s", opts.targetTier, name, hash)
}

// cleanupSpoolDir removes all files in the spool directory.
// Called on startup to clean up leftover files from previous runs.
func cleanupSpoolDir(spoolDir string) error {
	entries, err := os.ReadDir(spoolDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading spool directory: %w", err)
	}

	for _, entry := range entries {
		path := filepath.Join(spoolDir, entry.Name())
		if err := os.RemoveAll(path); err != nil { //nolint:gosec
			return fmt.Errorf("removing %q: %w", path, err)
		}
	}

	return nil
}
