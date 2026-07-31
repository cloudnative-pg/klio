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

package buffer

import "context"

// Handler is the interface used to process WAL data.
// This is vastly modeled around the pg_basebackup codebase.
type Handler interface {
	// HasWALFileOpened Checks whether there is a WAL file transmission opened
	HasWALFileOpened() bool

	// OpenWAL opens a new WAL for the passed position.
	// The passed position refers to the start of a WAL file
	OpenWAL(ctx context.Context, blockpos uint64) error

	// CloseWAL closes a WAL file
	CloseWAL(ctx context.Context) error

	// CurrentOffset returns the current offset in the WAL file
	CurrentOffset() (uint64, error)

	// Write writes data in the current WAL file
	Write(ctx context.Context, p []byte) (n int, err error)
}
