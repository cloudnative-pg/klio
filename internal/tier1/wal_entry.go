package tier1

import (
	"context"
	"os"
	"time"

	"github.com/kopia/kopia/fs"
	"github.com/mattetti/filebuffer"
)

const (
	postgresUID = 26
	postgresGID = 26
)

// WalEntry represents a WAL file entry
type WalEntry struct {
	walName string
	content []byte
}

// Open opens the WAL file entry
func (entry WalEntry) Open(_ context.Context) (fs.Reader, error) {
	return &walReader{
		Buffer: filebuffer.New(entry.content),
		entry:  entry,
	}, nil
}

// Name returns the name of the WAL file entry
func (entry WalEntry) Name() string {
	return entry.walName
}

// Size returns the size of the WAL file entry
func (entry WalEntry) Size() int64 {
	return int64(len(entry.content))
}

// Mode returns the mode of the WAL file entry
func (WalEntry) Mode() os.FileMode {
	return 0o600
}

// ModTime returns the modification time of the WAL file entry
func (WalEntry) ModTime() time.Time {
	return time.Now()
}

// IsDir returns false
func (WalEntry) IsDir() bool {
	return false
}

// Sys returns nil
func (WalEntry) Sys() any {
	return nil
}

// Owner returns the owner information of the WAL file entry
func (WalEntry) Owner() fs.OwnerInfo {
	return fs.OwnerInfo{
		UserID:  postgresUID,
		GroupID: postgresGID,
	}
}

// Device returns the device information of the WAL file entry
func (WalEntry) Device() fs.DeviceInfo {
	return fs.DeviceInfo{}
}

// LocalFilesystemPath returns an empty string
func (WalEntry) LocalFilesystemPath() string {
	return ""
}

// Close closes the WAL file entry
func (WalEntry) Close() {}

// WriteToFile writes the content of the WalEntry to a file.
func (entry WalEntry) WriteToFile(filePath string) error {
	file, err := os.Create(filePath) // nolint:gosec
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	_, err = file.Write(entry.content)
	if err != nil {
		return err
	}

	return nil
}

type walReader struct {
	*filebuffer.Buffer
	entry WalEntry
}

func (reader walReader) Entry() (fs.Entry, error) {
	return reader.entry, nil
}

func getWALFileEntry(walName string, content []byte) fs.File {
	return WalEntry{
		walName: walName,
		content: content,
	}
}
