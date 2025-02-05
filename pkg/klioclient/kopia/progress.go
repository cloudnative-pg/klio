package kopia

import (
	"log/slog"

	"github.com/kopia/kopia/snapshot/snapshotfs"

	"github.com/EnterpriseDB/klio/pkg/klioclient/common"
)

// kopiaProgress is the implementation of the Kopia progress.
type kopiaProgress struct {
	p   common.Progress
	log *slog.Logger

	startPath  string
	totalBytes int64
}

// CachedFile implements snapshotfs.UploadProgress.
func (k *kopiaProgress) CachedFile(path string, size int64) {
	k.log.Debug("using cached file", "path", path, "size", size)
}

// Enabled implements snapshotfs.UploadProgress.
func (k *kopiaProgress) Enabled() bool {
	return true
}

// Error implements snapshotfs.UploadProgress.
func (k *kopiaProgress) Error(_ string, _ error, _ bool) {}

// EstimatedDataSize implements snapshotfs.UploadProgress.
func (k *kopiaProgress) EstimatedDataSize(_ int64, totalBytes int64) {
	k.totalBytes = totalBytes
}

// EstimationParameters implements snapshotfs.UploadProgress.
func (k *kopiaProgress) EstimationParameters() snapshotfs.EstimationParameters {
	return snapshotfs.EstimationParameters{
		Type:              snapshotfs.EstimationTypeClassic,
		AdaptiveThreshold: snapshotfs.AdaptiveEstimationThreshold,
	}
}

// ExcludedDir implements snapshotfs.UploadProgress.
func (k *kopiaProgress) ExcludedDir(path string) {
	k.log.Debug("Excluded directory", "path", path)
}

// ExcludedFile implements snapshotfs.UploadProgress.
func (k *kopiaProgress) ExcludedFile(path string, size int64) {
	k.log.Debug("Excluded file", "path", path, "size", size)
}

// FinishedDirectory implements snapshotfs.UploadProgress.
func (k *kopiaProgress) FinishedDirectory(path string) {
	k.log.Debug("Finish directory", "path", path)
}

// FinishedFile implements snapshotfs.UploadProgress.
func (k *kopiaProgress) FinishedFile(_ string, _ error) {}

// FinishedHashingFile implements snapshotfs.UploadProgress.
func (k *kopiaProgress) FinishedHashingFile(_ string, _ int64) {}

// HashedBytes implements snapshotfs.UploadProgress.
func (k *kopiaProgress) HashedBytes(_ int64) {}

// HashingFile implements snapshotfs.UploadProgress.
func (k *kopiaProgress) HashingFile(_ string) {}

// StartedDirectory implements snapshotfs.UploadProgress.
func (k *kopiaProgress) StartedDirectory(path string) {
	k.log.Info("Start directory", "path", path)
}

// UploadFinished implements snapshotfs.UploadProgress.
func (k *kopiaProgress) UploadFinished() {
	k.p.NotifyFinish(k.startPath)
}

// UploadStarted implements snapshotfs.UploadProgress.
func (k *kopiaProgress) UploadStarted() {
	k.p.NotifyStart(k.startPath)
}

// UploadedBytes implements snapshotfs.UploadProgress.
func (k *kopiaProgress) UploadedBytes(numBytes int64) {
	k.p.NotifyStatus(k.startPath, uint64(numBytes), uint64(k.totalBytes))
}
