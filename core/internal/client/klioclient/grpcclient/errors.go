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

package grpcclient

import (
	"errors"
	"fmt"
)

// ErrInconsistentCertificate is raised when the server certificate cannot be parsed.
var ErrInconsistentCertificate = errors.New("inconsistent server certificate (parsing)")

// IncompleteWALFileError is raised when a WAL file has been uploaded incompletely.
type IncompleteWALFileError struct {
	uploadedSize uint64
	expectedSize uint64
}

// Error implements the error interface.
func (e *IncompleteWALFileError) Error() string {
	return fmt.Sprintf("uploaded %v expected %v", e.uploadedSize, e.expectedSize)
}
