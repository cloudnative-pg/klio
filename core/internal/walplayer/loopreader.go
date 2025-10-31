package walplayer

import "io"

// LoopReader implements io.Reader and loops over the provided buffer.
type LoopReader struct {
	buf []byte
	pos int
}

// NewLoopReader creates a new LoopReader.
func NewLoopReader(data []byte) *LoopReader {
	return &LoopReader{
		buf: data,
		pos: 0,
	}
}

// Read implements the io.Reader interface by cycling through the internal buffer.
// When the end of the buffer is reached, it wraps around to the beginning,
// allowing infinite reading from a finite buffer. Returns io.EOF only if the
// internal buffer is empty.
func (r *LoopReader) Read(p []byte) (int, error) {
	if len(r.buf) == 0 {
		return 0, io.EOF
	}

	n := 0
	for n < len(p) {
		remaining := len(r.buf) - r.pos
		toCopy := min(len(p)-n, remaining)

		copy(p[n:n+toCopy], r.buf[r.pos:r.pos+toCopy])
		r.pos = (r.pos + toCopy) % len(r.buf)
		n += toCopy
	}

	return n, nil
}
