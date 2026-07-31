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

package cmd

import (
	"testing"

	"github.com/cloudnative-pg/klio/core/pkg/config"
)

func TestDropTier2BaseURLWhenRecoveryDisabled(t *testing.T) {
	const tier2URL = "https://klio-server:51516"

	tests := []struct {
		name            string
		recoveryEnabled bool
		tier2URL        string
		wantDropped     bool
		wantTier2URL    string
	}{
		{
			name:            "recovery enabled keeps the tier2 base URL",
			recoveryEnabled: true,
			tier2URL:        tier2URL,
			wantDropped:     false,
			wantTier2URL:    tier2URL,
		},
		{
			name:            "recovery disabled drops the tier2 base URL",
			recoveryEnabled: false,
			tier2URL:        tier2URL,
			wantDropped:     true,
			wantTier2URL:    "",
		},
		{
			name:            "recovery disabled without a tier2 URL is a no-op",
			recoveryEnabled: false,
			tier2URL:        "",
			wantDropped:     false,
			wantTier2URL:    "",
		},
		{
			name:            "recovery enabled without a tier2 URL is a no-op",
			recoveryEnabled: true,
			tier2URL:        "",
			wantDropped:     false,
			wantTier2URL:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configuration := &config.Data{
				Tier2RecoveryEnabled: tt.recoveryEnabled,
			}
			configuration.Client.Base.Tier2URL = tt.tier2URL

			got := dropTier2BaseURLWhenRecoveryDisabled(configuration)
			if got != tt.wantDropped {
				t.Errorf("dropTier2BaseURLWhenRecoveryDisabled() = %v, want %v", got, tt.wantDropped)
			}
			if configuration.Client.Base.Tier2URL != tt.wantTier2URL {
				t.Errorf("Client.Base.Tier2URL = %q, want %q",
					configuration.Client.Base.Tier2URL, tt.wantTier2URL)
			}
		})
	}
}
