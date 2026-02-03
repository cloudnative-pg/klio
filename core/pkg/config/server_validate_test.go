package config

import (
	"testing"
)

func TestTLSConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  TLSConfig
		wantErr bool
	}{
		{"Valid", TLSConfig{TLSCert: "c", TLSKey: "k", ClientCACertFile: "ca"}, false},
		{"Missing Cert", TLSConfig{TLSKey: "k", ClientCACertFile: "ca"}, true},
		{"Missing All", TLSConfig{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.config.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTierConfigs_Validate(t *testing.T) {
	t.Run("Tier1", func(t *testing.T) {
		tests := []struct {
			name    string
			config  Tier1Config
			wantErr bool
		}{
			{
				name: "Valid",
				config: Tier1Config{
					EncryptionKey: "p",
					Base: BaseServerConfig{
						CacheDirectory:      "cache",
						RepositoryDirectory: "repo",
						ListenAddress:       "address",
					},
					Wal: WalServerConfig{
						ListenAddress: "address",
						WALPath:       "walPath",
					},
				},
				wantErr: false,
			},
			{"Missing EncryptionPassword", Tier1Config{}, true},
		}
		for _, tt := range tests {
			if err := tt.config.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("%s: Validate() error = %v", tt.name, err)
			}
		}
	})

	t.Run("Tier2", func(t *testing.T) {
		tests := []struct {
			name    string
			config  Tier2Config
			wantErr bool
		}{
			{"S3 Disabled (Valid)", Tier2Config{S3: S3Configuration{Enabled: false}}, false},
			{
				name: "S3 Enabled but Missing Fields",
				config: Tier2Config{
					S3: S3Configuration{Enabled: true, BucketName: "b", Endpoint: "e", Region: "r"},
					// Missing encryption password and addresses
				},
				wantErr: true,
			},
		}
		for _, tt := range tests {
			if err := tt.config.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("%s: Validate() error = %v", tt.name, err)
			}
		}
	})
}

func TestS3Configuration_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  S3Configuration
		wantErr bool
	}{
		{"Disabled (Empty allowed)", S3Configuration{Enabled: false}, false},
		{"Enabled and Valid", S3Configuration{Enabled: true, BucketName: "b", Endpoint: "e", Region: "r"}, false},
		{"Enabled and Valid without Endpoint", S3Configuration{Enabled: true, BucketName: "b", Region: "r"}, false},
		{"Enabled and Missing Fields", S3Configuration{Enabled: true, BucketName: "b"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.config.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestServerConfig_RequireTier2(t *testing.T) {
	tests := []struct {
		name    string
		config  ServerConfig
		wantErr bool
	}{
		{
			name: "Valid - Different cache directories",
			config: ServerConfig{
				TLS: TLSConfig{
					TLSCert:          "cert",
					TLSKey:           "key",
					ClientCACertFile: "ca",
				},
				Tier1: Tier1Config{
					Base: BaseServerConfig{
						CacheDirectory: "/cache/tier1",
					},
				},
				Tier2: Tier2Config{
					EncryptionKey:     "key",
					BaseListenAddress: "localhost:8080",
					WALListenAddress:  "localhost:8081",
					CacheDirectory:    "/cache/tier2",
					S3: S3Configuration{
						Enabled:    true,
						BucketName: "bucket",
						Endpoint:   "endpoint",
						Region:     "region",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Invalid - Same cache directories",
			config: ServerConfig{
				TLS: TLSConfig{
					TLSCert:          "cert",
					TLSKey:           "key",
					ClientCACertFile: "ca",
				},
				Tier1: Tier1Config{
					Base: BaseServerConfig{
						CacheDirectory: "/cache/shared",
					},
				},
				Tier2: Tier2Config{
					EncryptionKey:     "key",
					BaseListenAddress: "localhost:8080",
					WALListenAddress:  "localhost:8081",
					CacheDirectory:    "/cache/shared",
					S3: S3Configuration{
						Enabled:    true,
						BucketName: "bucket",
						Endpoint:   "endpoint",
						Region:     "region",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Invalid - S3 not enabled",
			config: ServerConfig{
				TLS: TLSConfig{
					TLSCert:          "cert",
					TLSKey:           "key",
					ClientCACertFile: "ca",
				},
				Tier1: Tier1Config{
					Base: BaseServerConfig{
						CacheDirectory: "/cache/tier1",
					},
				},
				Tier2: Tier2Config{
					CacheDirectory: "/cache/tier2",
					S3: S3Configuration{
						Enabled: false,
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.config.RequireTier2(); (err != nil) != tt.wantErr {
				t.Errorf("RequireTier2() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
