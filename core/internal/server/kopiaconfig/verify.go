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

package kopiaconfig

import (
	"context"
	"fmt"
	"os"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/kopia"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// VerifyTier1KopiaRepository verifies that a Tier1 Kopia repository can be accessed with the provided credentials.
func VerifyTier1KopiaRepository(ctx context.Context, cfg *config.Tier1Config) error {
	return verifyKopiaRepository(ctx, "tier1", func(tmpName string) error {
		return CreateTier1KopiaConfigFile(ctx, tmpName, cfg)
	})
}

// VerifyTier2KopiaRepository verifies that a Tier2 Kopia repository can be accessed with the provided credentials.
func VerifyTier2KopiaRepository(ctx context.Context, cfg *config.Tier2Config) error {
	return verifyKopiaRepository(ctx, "tier2", func(tmpName string) error {
		return CreateTier2KopiaConfigFile(ctx, tmpName, cfg, false)
	})
}

// verifyKopiaRepository is a helper that manages the lifecycle of a temporary configuration file.
// It handles the file creation, ensuring the file is closed and deleted after use.
func verifyKopiaRepository(ctx context.Context, tierLabel string, createKopiaConfigFile func(string) error) error {
	pattern := fmt.Sprintf("kopiaconfig_verify_%s_*", tierLabel)

	tmpFile, err := os.CreateTemp("", pattern)
	if err != nil {
		return fmt.Errorf("while creating temporary config file: %w", err)
	}

	tmpFileName := tmpFile.Name()
	_ = tmpFile.Close()

	defer kopia.CleanupConfigFile(ctx, tmpFileName)

	if err := createKopiaConfigFile(tmpFileName); err != nil {
		return fmt.Errorf("while verifying %s Kopia repository: %w", tierLabel, err)
	}

	return nil
}
