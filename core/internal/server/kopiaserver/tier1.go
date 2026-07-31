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

package kopiaserver

import (
	"context"
	"fmt"

	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// ServerControlCredential contains the credentials for Kopia server control operations.
type ServerControlCredential struct {
	// User is the username for server control authentication.
	User string

	// Password is the password for server control authentication.
	Password string
}

// StartTier1 runs a Tier 1 Kopia server.
func StartTier1(
	ctx context.Context,
	listenAddress string,
	tls *config.TLSConfig,
	serverControl ServerControlCredential,
	kopiaConfigFile string,
) error {
	kopiaCfg := Config{
		ListenAddress:         listenAddress,
		ServerControlUser:     serverControl.User,
		ServerControlPassword: serverControl.Password,
	}

	return start(ctx, kopiaConfigFile, &kopiaCfg, tls, opentelemetry.Tier1)
}

// InitializeTier1 initializes a new Kopia Tier1 Repository.
func InitializeTier1(ctx context.Context, cfg *config.Tier1Config) error {
	cacheDir := cfg.Base.CacheDirectory
	kopiaBinary, err := kopia.LookupBinary()
	if err != nil {
		return err
	}

	if err := kopia.CleanupCacheDirectory(cacheDir); err != nil {
		return err
	}

	opts := kopia.FSRepoOpts{
		CommonRepoOpts: kopia.CommonRepoOpts{
			KopiaBinary:        kopiaBinary,
			EncryptionPassword: cfg.EncryptionKey,
			CacheDirectory:     cacheDir,
		},
		DataDirectory: cfg.Base.RepositoryDirectory,
	}

	if err := kopia.InitializeFilesystem(ctx, opts); err != nil {
		return fmt.Errorf("while creating Kopia repository: %w", err)
	}

	return nil
}
