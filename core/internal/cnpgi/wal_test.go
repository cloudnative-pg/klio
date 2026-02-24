package cnpgi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetConnectionErrors(t *testing.T) {
	tests := []struct {
		name       string
		writeFile  bool
		configYAML string
		tier       string
		wantSubstr string
	}{
		{
			name:       "nonexistent config file",
			writeFile:  false,
			tier:       "tier1",
			wantSubstr: "while loading config file",
		},
		{
			name:       "invalid YAML",
			writeFile:  true,
			configYAML: "{invalid",
			tier:       "tier1",
			wantSubstr: "while decoding config file",
		},
		{
			name:      "unknown tier",
			writeFile: true,
			configYAML: `
client:
  cluster_name: test
  wal:
    address: "localhost:52000"
    tier2_address: "localhost:52001"
`,
			tier:       "tier3",
			wantSubstr: `unknown tier "tier3"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newGRPCClientManager()

			configPath := filepath.Join(t.TempDir(), "config.yaml")
			if tt.writeFile {
				if err := os.WriteFile(configPath, []byte(tt.configYAML), 0o600); err != nil {
					t.Fatalf("writing config file: %v", err)
				}
			}

			_, err := mgr.getConnection(restoreWALOptions{
				configFile: configPath,
				tier:       tt.tier,
			})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("error = %q, want substring %q", err, tt.wantSubstr)
			}
		})
	}
}
