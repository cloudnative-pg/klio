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
