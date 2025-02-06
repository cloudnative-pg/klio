package kopia

import (
	"log/slog"
	"sync/atomic"

	"github.com/kopia/kopia/snapshot/snapshotfs"

	"github.com/EnterpriseDB/klio/pkg/klioclient/common"
)

// kopiaProgress is the implementation of the Kopia progress.
type kopiaProgress struct {
	p   common.Progress
	log *slog.Logger

	startPath          string
	estimatedFileCount atomic.Int64
	estimatedBytes     atomic.Int64

	uploadedBytes atomic.Int64
	hashedBytes   atomic.Int64

	uploadedFiles atomic.Int64
	hashedFiles   atomic.Int64
}

// Enabled implements snapshotfs.UploadProgress.
func (k *kopiaProgress) Enabled() bool {
	return true
}

// EstimatedDataSize implements snapshotfs.UploadProgress.
func (k *kopiaProgress) EstimatedDataSize(fileCount int64, totalBytes int64) {
	k.estimatedFileCount.Store(fileCount)
	k.estimatedBytes.Store(totalBytes)
	k.log.Debug("received estimation", "fileCount", fileCount, "totalBytes", totalBytes)
}

// HashedBytes implements snapshotfs.UploadProgress.
func (k *kopiaProgress) HashedBytes(numBytes int64) {
	k.hashedBytes.Add(numBytes)
}

// UploadedBytes implements snapshotfs.UploadProgress.
func (k *kopiaProgress) UploadedBytes(numBytes int64) {
	k.uploadedBytes.Add(numBytes)
}

// EstimationParameters implements snapshotfs.UploadProgress.
func (k *kopiaProgress) EstimationParameters() snapshotfs.EstimationParameters {
	return snapshotfs.EstimationParameters{
		Type:              snapshotfs.EstimationTypeClassic,
		AdaptiveThreshold: snapshotfs.AdaptiveEstimationThreshold,
	}
}

// FinishedHashingFile implements snapshotfs.UploadProgress.
func (k *kopiaProgress) FinishedHashingFile(path string, numBytes int64) {
	k.log.Info("Finished hashing file", "path", path, "numBytes", numBytes)
	_ = k.hashedFiles.Add(1)
}

// CachedFile implements snapshotfs.UploadProgress.
func (k *kopiaProgress) CachedFile(path string, size int64) {
	k.log.Info("using cached file", "path", path, "size", size)
}

// Error implements snapshotfs.UploadProgress.
func (k *kopiaProgress) Error(path string, err error, isIgnored bool) {
	k.log.Warn("error while uploading file", "path", path, "err", err, "isIgnored", isIgnored)
}

// ExcludedDir implements snapshotfs.UploadProgress.
func (k *kopiaProgress) ExcludedDir(path string) {
	k.log.Debug("Excluded directory", "path", path)
}

// ExcludedFile implements snapshotfs.UploadProgress.
func (k *kopiaProgress) ExcludedFile(path string, size int64) {
	k.log.Info("Excluded file", "path", path, "size", size)
}

// FinishedDirectory implements snapshotfs.UploadProgress.
func (k *kopiaProgress) FinishedDirectory(path string) {
	k.log.Debug("Finish directory", "path", path)
}

// FinishedFile implements snapshotfs.UploadProgress.
func (k *kopiaProgress) FinishedFile(path string, err error) {
	k.log.Info("Finished uploading file", "path", path, "err", err)
	_ = k.uploadedFiles.Add(1)
}

// HashingFile implements snapshotfs.UploadProgress.
func (k *kopiaProgress) HashingFile(path string) {
	k.log.Info("Start hashing file", "path", path)
}

// StartedDirectory implements snapshotfs.UploadProgress.
func (k *kopiaProgress) StartedDirectory(path string) {
	k.log.Info(
		"Start directory",
		"path", path,
		"estimatedFileCount", k.estimatedFileCount.Load(),
		"estimatedBytes", k.estimatedBytes.Load(),
		"uploadedBytes", k.uploadedBytes.Load(),
		"hashedBytes", k.hashedBytes.Load(),
		"uploadedFiles", k.uploadedFiles.Load(),
		"hashedFiles", k.hashedFiles.Load(),
	)
}

// UploadFinished implements snapshotfs.UploadProgress.
func (k *kopiaProgress) UploadFinished() {
	k.p.NotifyFinish(k.startPath)
}

// UploadStarted implements snapshotfs.UploadProgress.
func (k *kopiaProgress) UploadStarted() {
	k.p.NotifyStart(k.startPath)
}
