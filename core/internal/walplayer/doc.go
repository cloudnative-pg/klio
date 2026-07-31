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

// Package walplayer provides functionality for generating and uploading WAL files to Klio.
//
// This package implements a WAL player system that can:
//   - Generate fake WAL files of configurable sizes using embedded template data
//   - Stream WAL files to a Klio server with concurrent workers
//   - Loop through template data to create arbitrarily large WAL files
//   - Collect and report statistics from WAL upload operations
//
// The main components include:
//   - Player: Orchestrates concurrent WAL file uploads to a Klio server
//   - WALWriter: Generates WAL files using looped template data
//   - LoopReader: Provides infinite reading from finite buffer data
//
// Example usage for generating WAL files:
//
//	writer := NewWALWriter(16) // 16MB segments
//	err := writer.ToDirectory(ctx, "/tmp/wal", 10) // Write 10 segments
//
// Example usage for playing WAL files:
//
//	player := walplayer.NewPlayer(workers, targetDirectory, blockSize*1024, &configuration.Client)
//	results := player.Play(ctx)
package walplayer
