package config

import (
	"strings"
	"testing"
)

func TestSourceConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  SourceConfig
		wantErr bool
		substr  string
	}{
		{
			name: "Valid config",
			config: SourceConfig{
				DSN:                          "postgres://...",
				StandardDSN:                  "postgres://...",
				Slot:                         "my_slot_123",
				StandbyMessageTimeoutSeconds: 10,
				FlushTimeoutMilliseconds:     100,
				BufferSize:                   1024,
			},
			wantErr: false,
		},
		{
			name: "Empty DSN",
			config: SourceConfig{
				Slot: "valid_slot",
			},
			wantErr: true,
			substr:  "dsn is empty",
		},
		{
			name: "Invalid Slot Name",
			config: SourceConfig{
				DSN:         "not_empty",
				StandardDSN: "not_empty",
				Slot:        "Invalid-Slot!",
			},
			wantErr: true,
			substr:  "slot name can only contain lower-case letters",
		},
		{
			name: "Standby Message Timeout too low",
			config: SourceConfig{
				DSN:                          "valid",
				StandardDSN:                  "valid",
				Slot:                         "valid",
				StandbyMessageTimeoutSeconds: 0,
				FlushTimeoutMilliseconds:     100,
				BufferSize:                   1024,
			},
			wantErr: true,
			substr:  "must be at least 1",
		},
		{
			name: "Flush Timeout too low",
			config: SourceConfig{
				DSN:                          "valid",
				StandardDSN:                  "valid",
				Slot:                         "valid",
				StandbyMessageTimeoutSeconds: 100,
				FlushTimeoutMilliseconds:     0,
				BufferSize:                   1024,
			},
			wantErr: true,
			substr:  "must be at least 1",
		},
		{
			name: "Buffer Size too low",
			config: SourceConfig{
				DSN:                          "valid",
				StandardDSN:                  "valid",
				Slot:                         "valid",
				StandbyMessageTimeoutSeconds: 100,
				FlushTimeoutMilliseconds:     200,
				BufferSize:                   0,
			},
			wantErr: true,
			substr:  "must be at least 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.substr != "" && !strings.Contains(err.Error(), tt.substr) {
				t.Errorf("Validate() error = %v, expected to contain %q", err, tt.substr)
			}
		})
	}
}

func TestWalRepositoryClientConfig_Validate(t *testing.T) {
	tests := []struct {
		name         string
		config       WalRepositoryClientConfig
		wantErr      bool
		expectedMsgs []string
	}{
		{
			name: "Valid with Primary Address",
			config: WalRepositoryClientConfig{
				Address:        "127.0.0.1:5432",
				ServerCertPath: "/certs/ca.crt",
				ClientCertPath: "/certs/client.crt",
				ClientKeyPath:  "/certs/client.key",
			},
			wantErr: false,
		},
		{
			name: "Valid with Tier2 Address only",
			config: WalRepositoryClientConfig{
				Tier2Address:   "10.0.0.5:5432",
				ServerCertPath: "/certs/ca.crt",
				ClientCertPath: "/certs/client.crt",
				ClientKeyPath:  "/certs/client.key",
			},
			wantErr: false,
		},
		{
			name: "Invalid: Both Addresses missing",
			config: WalRepositoryClientConfig{
				ServerCertPath: "/certs/ca.crt",
				ClientCertPath: "/certs/client.crt",
				ClientKeyPath:  "/certs/client.key",
			},
			wantErr:      true,
			expectedMsgs: []string{"at least one of address or tier2_address must be specified"},
		},
		{
			name: "Invalid: Missing all certificate paths",
			config: WalRepositoryClientConfig{
				Address: "127.0.0.1:5432",
			},
			wantErr: true,
			expectedMsgs: []string{
				"server_cert_path is empty",
				"client_cert_path is empty",
				"client_key_path is empty",
			},
		},
		{
			name:    "Invalid: Multiple failures (Join check)",
			config:  WalRepositoryClientConfig{},
			wantErr: true,
			expectedMsgs: []string{
				"at least one of address or tier2_address must be specified",
				"server_cert_path is empty",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr && err != nil {
				errMsg := err.Error()
				for _, expected := range tt.expectedMsgs {
					if !strings.Contains(errMsg, expected) {
						t.Errorf("expected error message to contain %q, but got %q", expected, errMsg)
					}
				}
			}
		})
	}
}

func TestBaseRepositoryClientConfig_Validate(t *testing.T) {
	tests := []struct {
		name         string
		config       BaseRepositoryClientConfig
		wantErr      bool
		expectedMsgs []string
	}{
		{
			name: "Valid with URL",
			config: BaseRepositoryClientConfig{
				URL:            "https://my.url.com",
				ServerCertPath: "/certs/ca.crt",
				ClientCertPath: "/certs/client.crt",
				ClientKeyPath:  "/certs/client.key",
			},
			wantErr: false,
		},
		{
			name: "Valid with Tier2 URL only",
			config: BaseRepositoryClientConfig{
				Tier2URL:       "https://my.tier2.url.com",
				ServerCertPath: "/certs/ca.crt",
				ClientCertPath: "/certs/client.crt",
				ClientKeyPath:  "/certs/client.key",
			},
			wantErr: false,
		},
		{
			name: "Invalid: Both URLs missing",
			config: BaseRepositoryClientConfig{
				ServerCertPath: "/certs/ca.crt",
				ClientCertPath: "/certs/client.crt",
				ClientKeyPath:  "/certs/client.key",
			},
			wantErr:      true,
			expectedMsgs: []string{"at least one of url or tier2_url must be specified"},
		},
		{
			name: "Invalid: Missing all certificate paths",
			config: BaseRepositoryClientConfig{
				URL: "https://my.url.com",
			},
			wantErr: true,
			expectedMsgs: []string{
				"server_cert_path is empty",
				"client_cert_path is empty",
				"client_key_path is empty",
			},
		},
		{
			name: "Invalid: Partial certificates missing",
			config: BaseRepositoryClientConfig{
				URL:            "https://primary.repo.com",
				ServerCertPath: "/certs/ca.crt",
				// Client paths missing
			},
			wantErr: true,
			expectedMsgs: []string{
				"client_cert_path is empty",
				"client_key_path is empty",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr && err != nil {
				errMsg := err.Error()
				for _, expected := range tt.expectedMsgs {
					if !strings.Contains(errMsg, expected) {
						t.Errorf("expected error message to contain %q, but got %q", expected, errMsg)
					}
				}
			}
		})
	}
}

func TestClientConfig_Validate(t *testing.T) {
	tests := []struct {
		name         string
		config       ClientConfig
		wantErr      bool
		expectedMsgs []string
	}{
		{
			name: "Valid config",
			config: ClientConfig{
				ClusterName: "my-cluster",
				Base: BaseRepositoryClientConfig{
					URL:            "https://my.url.com",
					ServerCertPath: "/certs/ca.crt",
					ClientCertPath: "/certs/client.crt",
					ClientKeyPath:  "/certs/client.key",
				},
				Wal: WalRepositoryClientConfig{
					Address:        "127.0.0.1:5432",
					ServerCertPath: "/certs/ca.crt",
					ClientCertPath: "/certs/client.crt",
					ClientKeyPath:  "/certs/client.key",
				},
			},
			wantErr: false,
		},
		{
			name: "Invalid: ClusterName empty",
			config: ClientConfig{
				Base: BaseRepositoryClientConfig{
					URL:            "https://my.url.com",
					ServerCertPath: "/certs/ca.crt",
					ClientCertPath: "/certs/client.crt",
					ClientKeyPath:  "/certs/client.key",
				},
			},
			wantErr:      true,
			expectedMsgs: []string{"cluster_name is empty"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr && err != nil {
				errMsg := err.Error()
				for _, expected := range tt.expectedMsgs {
					if !strings.Contains(errMsg, expected) {
						t.Errorf("expected error message to contain %q, but got %q", expected, errMsg)
					}
				}
			}
		})
	}
}
