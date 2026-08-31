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

package server

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/cloudnative-pg/machinery/pkg/log"
)

// migrationSuffix is appended to the destination to stage a migration. Staging
// next to the destination keeps the final step a rename within the same
// filesystem, so an interrupted copy never leaves a partial queue in place.
const migrationSuffix = ".migrating"

// MigrateQueueDirectory moves the NATS work queue to destination when the
// dedicated queue volume was added to, or removed from, the Server resource.
// The two live on different volumes, so the queue is copied and then removed.
// A non-empty destination always wins, as overwriting it would discard the
// tasks the server is about to resume.
func MigrateQueueDirectory(ctx context.Context, source, destination string) error {
	contextLogger := log.FromContext(ctx)

	needed, err := migrationNeeded(ctx, source, destination)
	if err != nil || !needed {
		return err
	}

	contextLogger.Info("Migrating the work queue to its new location",
		"source", source,
		"destination", destination,
	)

	if err := copyQueue(source, destination); err != nil {
		return err
	}

	if err := os.RemoveAll(source); err != nil {
		return fmt.Errorf("while removing the previous queue location %q: %w", source, err)
	}

	contextLogger.Info("Work queue migrated", "destination", destination)

	return nil
}

// migrationNeeded tells whether the queue has to be moved.
func migrationNeeded(ctx context.Context, source, destination string) (bool, error) {
	if source == "" || source == destination {
		return false, nil
	}

	sourceEmpty, err := isEmptyDir(source)
	if err != nil {
		return false, fmt.Errorf("while inspecting queue migration source %q: %w", source, err)
	}
	if sourceEmpty {
		return false, nil
	}

	destinationEmpty, err := isEmptyDir(destination)
	if err != nil {
		return false, fmt.Errorf("while inspecting queue migration destination %q: %w", destination, err)
	}
	if !destinationEmpty {
		log.FromContext(ctx).Warning(
			"Queue data found in both the previous and the current location, keeping the current one. "+
				"The previous location can be removed manually once inspected",
			"source", source,
			"destination", destination,
		)

		return false, nil
	}

	return true, nil
}

// copyQueue copies the queue into destination through the staging directory.
func copyQueue(source, destination string) error {
	staging := destination + migrationSuffix
	if err := os.RemoveAll(staging); err != nil {
		return fmt.Errorf("while clearing queue migration staging directory %q: %w", staging, err)
	}

	if err := os.CopyFS(staging, os.DirFS(source)); err != nil {
		// Leaving a partial copy behind would waste space on a volume that may
		// well be the one running out of it.
		_ = os.RemoveAll(staging)

		return fmt.Errorf("while copying the work queue from %q to %q: %w", source, staging, err)
	}

	// The caller is about to delete the source, so the copy has to be on disk
	// rather than in the page cache.
	if err := syncTree(staging); err != nil {
		return fmt.Errorf("while flushing the copied work queue: %w", err)
	}

	// The destination is empty, but it exists as soon as its volume is
	// mounted, and rename refuses to replace an existing directory.
	if err := os.RemoveAll(destination); err != nil {
		return fmt.Errorf("while clearing queue destination %q: %w", destination, err)
	}

	if err := os.Rename(staging, destination); err != nil {
		return fmt.Errorf("while moving %q to %q: %w", staging, destination, err)
	}

	if err := syncDir(filepath.Dir(destination)); err != nil {
		return fmt.Errorf("while flushing %q: %w", filepath.Dir(destination), err)
	}

	return nil
}

// isEmptyDir reports whether the given path holds no entries. A missing path
// counts as empty.
func isEmptyDir(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}

		return false, err
	}

	return len(entries) == 0, nil
}

// syncTree fsyncs every file and directory of the tree rooted at root.
func syncTree(root string) error {
	return filepath.WalkDir(root, func(currentPath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			return syncDir(currentPath)
		}

		file, err := os.Open(currentPath) //nolint:gosec // the tree is the queue directory we just wrote
		if err != nil {
			return err
		}
		defer func() {
			_ = file.Close()
		}()

		return file.Sync()
	})
}

// syncDir fsyncs a directory, so the names it holds survive a crash.
func syncDir(path string) error {
	dir, err := os.Open(path) //nolint:gosec // the path is a queue directory from the server configuration
	if err != nil {
		return err
	}
	defer func() {
		_ = dir.Close()
	}()

	return dir.Sync()
}
