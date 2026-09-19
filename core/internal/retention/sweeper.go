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

// Package retention periodically evaluates and enforces backup retention
// across every cluster in the repository.
package retention

import (
	"fmt"

	klioclientkopia "github.com/cloudnative-pg/klio/core/internal/client/klioclient/kopia"
	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/internal/repository"
)

// Options configures a Sweeper.
type Options struct {
	// Tier1KopiaConfig is a Kopia config file connected directly to the
	// tier1 repository (see the package doc).
	Tier1KopiaConfig string

	// Tier1ServerAddress and Tier1ServerCertificateFingerprint are used to
	// refresh the tier1 Kopia server's cache after a sweep pass deletes a
	// backup.
	Tier1ServerAddress                string
	Tier1ServerCertificateFingerprint string

	// Tier2KopiaConfig is a Kopia config file connected directly to the
	// tier2 repository. Empty when tier2 is not configured.
	Tier2KopiaConfig                  string
	Tier2ServerAddress                string
	Tier2ServerCertificateFingerprint string

	// RunID and RunSecret authenticate the Kopia server refresh calls.
	RunID     string
	RunSecret string

	// Tier1WALRepository is where the retention policy is stored (see
	// walserver.GetClusterRetentionPolicy) and where tier1 WAL retention is
	// applied.
	Tier1WALRepository *repository.Connection

	// Tier2WALRepository is where tier2 WAL retention is applied. Nil when
	// tier2 is not configured.
	Tier2WALRepository *repository.Connection
}

// Sweeper evaluates and enforces retention for every cluster in the
// repository, for tier1 and (when configured) tier2.
type Sweeper struct {
	opts *Options

	tier1Kopia  *kopia.Client
	tier1Client *klioclientkopia.Connection

	tier2Kopia   *kopia.Client
	tier2Client  *klioclientkopia.Connection
	tier2Enabled bool
}

// NewSweeper builds a Sweeper connected directly to the tier1 (and,
// when configured, tier2) repositories.
func NewSweeper(opts *Options) (*Sweeper, error) {
	tier1Client, err := klioclientkopia.FromKopiaConfig(opts.Tier1KopiaConfig)
	if err != nil {
		return nil, fmt.Errorf("while creating tier1 client: %w", err)
	}

	s := &Sweeper{
		opts:        opts,
		tier1Kopia:  tier1Client.KopiaClient(),
		tier1Client: tier1Client,
	}

	if opts.Tier2KopiaConfig != "" {
		tier2Client, err := klioclientkopia.FromKopiaConfig(opts.Tier2KopiaConfig)
		if err != nil {
			return nil, fmt.Errorf("while creating tier2 client: %w", err)
		}

		s.tier2Kopia = tier2Client.KopiaClient()
		s.tier2Client = tier2Client
		s.tier2Enabled = true
	}

	return s, nil
}
