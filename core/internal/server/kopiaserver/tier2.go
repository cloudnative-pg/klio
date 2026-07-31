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

// StartTier2 runs a Tier 2 Kopia server.
func StartTier2(
	ctx context.Context,
	listenAddress string,
	tls *config.TLSConfig,
	serverControl ServerControlCredential,
	kopiaConfigFile string,
) error {
	kopiaServerConfig := Config{
		ListenAddress:         listenAddress,
		ServerControlUser:     serverControl.User,
		ServerControlPassword: serverControl.Password,
	}

	return start(ctx, kopiaConfigFile, &kopiaServerConfig, tls, opentelemetry.Tier2)
}

// InitializeTier2 initializes a new Kopia Tier2 Repository.
func InitializeTier2(ctx context.Context, cfg *config.Tier2Config) error {
	cacheDir := cfg.CacheDirectory
	if err := kopia.CleanupCacheDirectory(cacheDir); err != nil {
		return err
	}

	kopiaBinary, err := kopia.LookupBinary()
	if err != nil {
		return err
	}

	if err := kopia.InitializeS3(ctx, kopia.S3RepoOpts{
		CommonRepoOpts: kopia.CommonRepoOpts{
			KopiaBinary:        kopiaBinary,
			EncryptionPassword: cfg.EncryptionKey,
			PersistCredentials: false,
			CacheDirectory:     cacheDir,
		},
		BucketName:         cfg.S3.BucketName,
		Endpoint:           cfg.S3.Endpoint,
		Region:             cfg.S3.Region,
		Prefix:             cfg.S3.Prefix,
		AccessKeyID:        cfg.S3.AccessKeyID,
		SecretAccessKey:    cfg.S3.SecretAccessKey,
		SessionToken:       cfg.S3.SessionToken,
		CustomCABundleFile: cfg.S3.CustomCABundleFile,
	}); err != nil {
		return fmt.Errorf("while creating Kopia repository: %w", err)
	}

	return nil
}
