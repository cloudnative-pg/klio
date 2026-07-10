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
