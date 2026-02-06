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
