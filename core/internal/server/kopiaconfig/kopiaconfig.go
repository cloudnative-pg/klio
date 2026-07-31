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

	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// CreateTier1KopiaConfigFile creates a Kopia config file for tier1.
func CreateTier1KopiaConfigFile(ctx context.Context, fileName string, cfg *config.Tier1Config) error {
	cacheDir := cfg.Base.CacheDirectory

	kopiaBinary, err := kopia.LookupBinary()
	if err != nil {
		return err
	}

	if err := kopia.ConnectFileSystem(ctx, fileName, kopia.FSRepoOpts{
		CommonRepoOpts: kopia.CommonRepoOpts{
			KopiaBinary:        kopiaBinary,
			EncryptionPassword: cfg.EncryptionKey,
			PersistCredentials: true,
			CacheDirectory:     cacheDir,
		},
		DataDirectory: cfg.Base.RepositoryDirectory,
	}); err != nil {
		return fmt.Errorf("while connecting to Kopia repository: %w", err)
	}

	return nil
}

// CreateTier2KopiaConfigFile creates a Kopia config file for tier2.
// When readOnly is true, the repository connection is configured as read-only,
// which prevents any write operations through this config file.
func CreateTier2KopiaConfigFile(ctx context.Context, fileName string, cfg *config.Tier2Config, readOnly bool) error {
	cacheDir := cfg.CacheDirectory

	kopiaBinary, err := kopia.LookupBinary()
	if err != nil {
		return err
	}

	if err := kopia.ConnectS3(ctx, fileName, kopia.S3RepoOpts{
		CommonRepoOpts: kopia.CommonRepoOpts{
			KopiaBinary:        kopiaBinary,
			EncryptionPassword: cfg.EncryptionKey,
			PersistCredentials: true,
			CacheDirectory:     cacheDir,
			ReadOnly:           readOnly,
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
		return fmt.Errorf("while connecting to Kopia repository: %w", err)
	}

	return nil
}
