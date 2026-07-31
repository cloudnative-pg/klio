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

package klioclient

import "errors"

// WALNotFoundError is returned when the WAL file is not found.
type WALNotFoundError struct {
	WalName string
}

func (e *WALNotFoundError) Error() string {
	return "WAL file not found: " + e.WalName
}

// ErrMissingWALFile is raised when the client requires a WAL file
// that doesn't exist on the server.
var ErrMissingWALFile = errors.New("non existing WAL file")

// IncompleteTransmissionError is raised when downloading a WAL file
// from a Klio server and the transmission got interrupted after having
// received a correct block of data.
// This usually happens when a WAL file that is being written server-side
// is being read.
type IncompleteTransmissionError struct {
	// Inner is the underlying error
	Inner error

	// WrittenBytes is the number of bytes that have successfully beings received
	// by the server
	WrittenBytes int
}

// Error implements the error interface.
func (e IncompleteTransmissionError) Error() string {
	return "incomplete WAL file received: " + e.Inner.Error()
}

// Unwrap implements the error interface.
func (e IncompleteTransmissionError) Unwrap() error {
	return e.Inner
}
