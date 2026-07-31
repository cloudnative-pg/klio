/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

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
