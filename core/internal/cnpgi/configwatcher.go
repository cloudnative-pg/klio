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

package cnpgi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// ErrConfigFileChanged is returned when the config file changes on disk.
var ErrConfigFileChanged = errors.New("config file changed, restarting")

// NewConfigFileWatcher creates a manager.RunnableFunc that polls a config file,
// or every file in a config directory, and returns an error when the content
// changes. This causes the manager to shut down, and the kubelet will restart
// the container.
//
// Polling is used instead of fsnotify because Kubernetes updates secret
// volumes via symlink swaps, which fsnotify may not detect reliably.
func NewConfigFileWatcher(
	configPath string,
	interval time.Duration,
) manager.RunnableFunc {
	return func(ctx context.Context) error {
		logger := log.FromContext(ctx).WithName("config-watcher")

		initialHash, err := hashPath(configPath)
		if err != nil {
			return fmt.Errorf("while reading initial config: %w", err)
		}

		logger.Info("Config watcher started", "path", configPath)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				currentHash, err := hashPath(configPath)
				if err != nil {
					logger.Error(err, "Failed to read config, will retry")
					continue
				}

				if currentHash != initialHash {
					logger.Info("Config changed, shutting down for restart",
						"path", configPath)
					return ErrConfigFileChanged
				}
			}
		}
	}
}

// hashPath hashes a file, or every regular file directly inside a directory
// together with its name. Directory entries are resolved through symlinks so
// the projected Secret volume layout (`<key>` linking into `..data/`) is
// followed, while its internal directories are skipped.
func hashPath(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("while reading path %q: %w", path, err)
	}
	if !info.IsDir() {
		return hashFile(path)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("while reading directory %q: %w", path, err)
	}

	hash := sha256.New()
	for _, entry := range entries {
		entryPath := filepath.Join(path, entry.Name())
		entryInfo, err := os.Stat(entryPath)
		if err != nil {
			return "", fmt.Errorf("while reading path %q: %w", entryPath, err)
		}
		if entryInfo.IsDir() {
			continue
		}

		fileHash, err := hashFile(entryPath)
		if err != nil {
			return "", err
		}
		hash.Write([]byte(entry.Name()))
		hash.Write([]byte(fileHash))
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// hashFile reads a file and returns its SHA256 hash as a hex string.
func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return "", fmt.Errorf("while reading file %q: %w", path, err)
	}

	hash := sha256.Sum256(data)

	return hex.EncodeToString(hash[:]), nil
}
