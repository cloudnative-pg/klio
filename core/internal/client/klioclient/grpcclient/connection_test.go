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

package grpcclient

import (
	"context"
	"fmt"
	"testing"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spf13/afero"

	"github.com/cloudnative-pg/klio/core/internal/repository"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

func BenchmarkLookupSnapshotsViaKlioServer(b *testing.B) {
	createTemporaryKlioRepo := func(ctx context.Context) (*TemporaryConnection, error) {
		conn, err := ConnectTemporary(
			ctx,
			log.GetLogger(),
			&config.ClientConfig{
				ClusterName: "cluster-name",
			},
			repository.Options{
				FS:       afero.NewMemMapFs(),
				Password: "random-string",
			},
		)
		if err != nil {
			return nil, fmt.Errorf("error while creating local kopia repository: %w", err)
		}

		return conn, nil
	}

	BenchLookupSnapshots(b, createTemporaryKlioRepo)
}
