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

type walEntry struct {
	walName string
	content []byte
}

func (entry walEntry) Open(_ context.Context) (fs.Reader, error) {
	return &walReader{
		Buffer: filebuffer.New(entry.content),
		entry:  entry,
	}, nil
}

func (entry walEntry) Name() string {
	return entry.walName
}

func (entry walEntry) Size() int64 {
	return int64(len(entry.content))
}

func (walEntry) Mode() os.FileMode {
	return 0o600
}

func (walEntry) ModTime() time.Time {
	return time.Now()
}

func (walEntry) IsDir() bool {
	return false
}

func (walEntry) Sys() any {
	return nil
}

func (walEntry) Owner() fs.OwnerInfo {
	return fs.OwnerInfo{
		UserID:  postgresUID,
		GroupID: postgresGID,
	}
}

func (walEntry) Device() fs.DeviceInfo {
	return fs.DeviceInfo{}
}

func (walEntry) LocalFilesystemPath() string {
	return ""
}

func (walEntry) Close() {}

type walReader struct {
	*filebuffer.Buffer
	entry walEntry
}

func (reader walReader) Entry() (fs.Entry, error) {
	return reader.entry, nil
}

func getWALFileEntry(walName string, content []byte) fs.File {
	return walEntry{
		walName: walName,
		content: content,
	}
}
