package types

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/kopia/kopia/fs"
	"github.com/mattetti/filebuffer"
)

const (
	postgresUID = 26
	postgresGID = 26
)

// Entry represents a WAL file entry.
type Entry struct {
	walName string
	content []byte
}

// NewEntry creates a new WAL entry.
func NewEntry(name string, content []byte) *Entry {
	return &Entry{
		walName: name,
		content: content,
	}
}

// Open opens the WAL file entry.
func (entry Entry) Open(_ context.Context) (fs.Reader, error) { //nolint:ireturn
	return &entryReader{
		Buffer: filebuffer.New(entry.content),
		entry:  entry,
	}, nil
}

// Name returns the name of the WAL file entry.
func (entry Entry) Name() string {
	return entry.walName
}

// Size returns the size of the WAL file entry.
func (entry Entry) Size() int64 {
	return int64(len(entry.content))
}

// Content returns the content of the WAL file entry.
func (entry Entry) Content() []byte {
	return entry.content
}

// Mode returns the mode of the WAL file entry.
func (Entry) Mode() os.FileMode {
	return 0o600
}

// ModTime returns the modification time of the WAL file entry.
func (Entry) ModTime() time.Time {
	return time.Now()
}

// IsDir returns false.
func (Entry) IsDir() bool {
	return false
}

// Sys returns nil.
func (Entry) Sys() any {
	return nil
}

// Owner returns the owner information of the WAL file entry.
func (Entry) Owner() fs.OwnerInfo {
	return fs.OwnerInfo{
		UserID:  postgresUID,
		GroupID: postgresGID,
	}
}

// Device returns the device information of the WAL file entry.
func (Entry) Device() fs.DeviceInfo {
	return fs.DeviceInfo{}
}

// LocalFilesystemPath returns an empty string.
func (Entry) LocalFilesystemPath() string {
	return ""
}

// Close closes the WAL file entry.
func (Entry) Close() {}

// WriteToFile writes the content of the WalEntry to a file.
func (entry Entry) WriteToFile(filePath string) error {
	file, err := os.Create(filePath) //nolint:gosec
	if err != nil {
		return fmt.Errorf("while creating file %s: %w", filePath, err)
	}
	defer func() { _ = file.Close() }()

	_, err = file.Write(entry.content)
	if err != nil {
		return fmt.Errorf("while writing file %s: %w", filePath, err)
	}

	return nil
}

type entryReader struct {
	*filebuffer.Buffer
	entry Entry
}

func (reader entryReader) Entry() (fs.Entry, error) { //nolint:ireturn
	return reader.entry, nil
}
