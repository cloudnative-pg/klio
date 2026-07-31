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

package kopia

import (
	"fmt"
	"os"
	"path/filepath"
)

// CleanupCacheDirectory removes the cache directory at the specified path.
func CleanupCacheDirectory(name string) error {
	cleanPath := filepath.Clean(name)

	if !filepath.IsAbs(cleanPath) {
		return fmt.Errorf("cache directory %q must be an absolute path", cleanPath)
	}

	if cleanPath == string(filepath.Separator) {
		return fmt.Errorf("cannot clean root directory %q", cleanPath)
	}

	if err := os.RemoveAll(cleanPath); err != nil {
		return fmt.Errorf("while cleaning up cache directory %q: %w", cleanPath, err)
	}

	return nil
}
