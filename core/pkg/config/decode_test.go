package config

import (
	"strings"
	"testing"
)

func TestDecodeYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    Data
		wantErr bool
	}{
		{
			name: "full client config",
			yaml: `
client:
  cluster_name: my-cluster
  wal:
    address: "klio-server:52000"
    tier2_address: "klio-server:52001"
    server_cert_path: /certs/wal-server.crt
    client_cert_path: /certs/wal-client.crt
    client_key_path: /certs/wal-client.key
  base:
    url: "https://klio-server:51515"
    tier2_url: "https://klio-server:51516"
    server_cert_path: /certs/base-server.crt
    client_cert_path: /certs/base-client.crt
    client_key_path: /certs/base-client.key
`,
			want: Data{
				Client: ClientConfig{
					ClusterName: "my-cluster",
					Wal: WalRepositoryClientConfig{
						Address:        "klio-server:52000",
						Tier2Address:   "klio-server:52001",
						ServerCertPath: "/certs/wal-server.crt",
						ClientCertPath: "/certs/wal-client.crt",
						ClientKeyPath:  "/certs/wal-client.key",
					},
					Base: BaseRepositoryClientConfig{
						URL:            "https://klio-server:51515",
						Tier2URL:       "https://klio-server:51516",
						ServerCertPath: "/certs/base-server.crt",
						ClientCertPath: "/certs/base-client.crt",
						ClientKeyPath:  "/certs/base-client.key",
					},
				},
			},
		},
		{
			name: "source config",
			yaml: `
source:
  dsn: "postgres://localhost:5432/mydb"
  standard_dsn: "postgres://localhost:5432/mydb"
  slot: my_slot
  standby_message_timeout_seconds: 15
  flush_timeout_ms: 300
  buffer_size: 4096
`,
			want: Data{
				Source: SourceConfig{
					DSN:                          "postgres://localhost:5432/mydb",
					StandardDSN:                  "postgres://localhost:5432/mydb",
					Slot:                         "my_slot",
					StandbyMessageTimeoutSeconds: 15,
					FlushTimeoutMilliseconds:     300,
					BufferSize:                   4096,
				},
			},
		},
		{
			name: "empty YAML produces zero-value Data",
			yaml: `{}`,
			want: Data{},
		},
		{
			name:    "invalid YAML",
			yaml:    `{invalid`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeYAML(strings.NewReader(tt.yaml))

			if (err != nil) != tt.wantErr {
				t.Fatalf("DecodeYAML() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			if got.Client != tt.want.Client {
				t.Errorf("Client = %+v, want %+v", got.Client, tt.want.Client)
			}
			if got.Source != tt.want.Source {
				t.Errorf("Source = %+v, want %+v", got.Source, tt.want.Source)
			}
			if got.Tier1RetentionPolicy != tt.want.Tier1RetentionPolicy {
				t.Errorf("Tier1RetentionPolicy = %v, want %v",
					got.Tier1RetentionPolicy, tt.want.Tier1RetentionPolicy)
			}
			if got.Tier2RetentionPolicy != tt.want.Tier2RetentionPolicy {
				t.Errorf("Tier2RetentionPolicy = %v, want %v",
					got.Tier2RetentionPolicy, tt.want.Tier2RetentionPolicy)
			}
		})
	}
}
