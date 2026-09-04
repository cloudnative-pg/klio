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

package config

import (
	"errors"
	"testing"
)

func TestTLSConfigValidate(t *testing.T) {
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

func TestTierConfigsValidate(t *testing.T) {
	t.Run("Tier1", func(t *testing.T) {
		tests := []struct {
			name    string
			config  Tier1Config
			wantErr bool
		}{
			{
				name: "Valid",
				config: Tier1Config{
					EncryptionKeyFile: "/path/to/key",
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
			{"Missing EncryptionKeyFile", Tier1Config{}, true},
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
					S3: S3Configuration{
						Enabled:         true,
						BucketName:      "b",
						Endpoint:        "e",
						Region:          "r",
						AccessKeyID:     "key",
						SecretAccessKey: "secret",
					},
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

func TestTierConfigCompressionValidate(t *testing.T) {
	validTier1 := func() Tier1Config {
		return Tier1Config{
			EncryptionKeyFile: "/path/to/key",
			Base: BaseServerConfig{
				CacheDirectory:      "cache",
				RepositoryDirectory: "repo",
				ListenAddress:       "address",
			},
			Wal: WalServerConfig{
				ListenAddress: "address",
				WALPath:       "walPath",
			},
		}
	}

	t.Run("Tier1 valid compression", func(t *testing.T) {
		cfg := validTier1()
		cfg.Compression = CompressionServerConfig{Algorithm: "zstd"}
		if err := cfg.Validate(); err != nil {
			t.Errorf("Validate() unexpected error = %v", err)
		}
	})

	t.Run("Tier1 empty compression", func(t *testing.T) {
		cfg := validTier1()
		if err := cfg.Validate(); err != nil {
			t.Errorf("Validate() unexpected error = %v", err)
		}
	})

	t.Run("Tier1 invalid compression", func(t *testing.T) {
		cfg := validTier1()
		cfg.Compression = CompressionServerConfig{Algorithm: "bogus"}
		if err := cfg.Validate(); err == nil {
			t.Errorf("Validate() expected an error for invalid compression")
		}
	})

	t.Run("Tier1 valid compression size range", func(t *testing.T) {
		cfg := validTier1()
		cfg.Compression = CompressionServerConfig{
			Algorithm: "zstd", MinSize: 4096, MaxSize: 1048576,
		}
		if err := cfg.Validate(); err != nil {
			t.Errorf("Validate() unexpected error = %v", err)
		}
	})

	t.Run("Tier1 inconsistent compression size range", func(t *testing.T) {
		cfg := validTier1()
		cfg.Compression = CompressionServerConfig{
			Algorithm: "zstd", MinSize: 1048576, MaxSize: 4096,
		}
		if err := cfg.Validate(); !errors.Is(err, ErrInvalidCompressionSizeRange) {
			t.Errorf("Validate() error = %v, want ErrInvalidCompressionSizeRange", err)
		}
	})

	t.Run("Tier2 inconsistent compression size range", func(t *testing.T) {
		cfg := Tier2Config{
			S3: S3Configuration{Enabled: false},
			Compression: CompressionServerConfig{
				Algorithm: "zstd", MinSize: 1048576, MaxSize: 4096,
			},
		}
		if err := cfg.Validate(); !errors.Is(err, ErrInvalidCompressionSizeRange) {
			t.Errorf("Validate() error = %v, want ErrInvalidCompressionSizeRange", err)
		}
	})

	t.Run("Tier2 invalid compression", func(t *testing.T) {
		cfg := Tier2Config{
			S3:          S3Configuration{Enabled: false},
			Compression: CompressionServerConfig{Algorithm: "bogus"},
		}
		if err := cfg.Validate(); err == nil {
			t.Errorf("Validate() expected an error for invalid compression")
		}
	})
}

func TestS3ConfigurationValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  S3Configuration
		wantErr bool
	}{
		{"Disabled (Empty allowed)", S3Configuration{Enabled: false}, false},
		{
			"Enabled with explicit credentials (Valid)",
			S3Configuration{
				Enabled:         true,
				BucketName:      "b",
				Endpoint:        "e",
				Region:          "r",
				AccessKeyID:     "key",
				SecretAccessKey: "secret",
			},
			false,
		},
		{"Enabled but missing bucket and region", S3Configuration{Enabled: true}, true},
		{
			"Enabled without credentials - uses IAM role (Valid)",
			S3Configuration{
				Enabled:    true,
				BucketName: "b",
				Region:     "r",
			},
			false,
		},
		{
			"Partial credentials - only access key (Invalid)",
			S3Configuration{
				Enabled:     true,
				BucketName:  "b",
				Endpoint:    "e",
				Region:      "r",
				AccessKeyID: "key",
			},
			true,
		},
		{
			"Partial credentials - only secret key (Invalid)",
			S3Configuration{
				Enabled:         true,
				BucketName:      "b",
				Endpoint:        "e",
				Region:          "r",
				SecretAccessKey: "secret",
			},
			true,
		},
		{
			"Without credentials with endpoint (Valid)",
			S3Configuration{
				Enabled:    true,
				BucketName: "b",
				Endpoint:   "e",
				Region:     "r",
			},
			false,
		},
		{
			"Explicit credentials without endpoint (Valid - endpoint optional)",
			S3Configuration{
				Enabled:         true,
				BucketName:      "b",
				Region:          "r",
				AccessKeyID:     "key",
				SecretAccessKey: "secret",
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.config.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestServerConfigRequireTier2(t *testing.T) {
	tests := []struct {
		name    string
		config  ServerConfig
		wantErr bool
	}{
		{
			name: "Valid - Different cache directories with explicit credentials",
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
					EncryptionKeyFile: "/path/to/key",
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
					EncryptionKeyFile: "/path/to/key",
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
