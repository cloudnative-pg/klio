package buffer

// Handler is the interface used to process WAL data.
// This is vastly modeled around the pg_basebackup codebase.
type Handler interface {
	// HasWALFileOpened Checks whether there is a WAL file transmission opened
	HasWALFileOpened() bool

	// OpenWAL opens a new WAL for the passed position.
	// The passed position refers to the start of a WAL file
	OpenWAL(blockpos uint64) error

	// CloseWAL closes a WAL file
	CloseWAL() error

	// CurrentOffset returns the current offset in the WAL file
	CurrentOffset() uint64

	// Write writes data in the current WAL file
	Write(p []byte) (n int, err error)
}
