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

func TestIsValidCompressionAlgorithm(t *testing.T) {
	tests := []struct {
		algorithm string
		want      bool
	}{
		{"none", true},
		{"zstd", true},
		{"zstd-fastest", true},
		{"zstd-better-compression", true},
		{"s2-default", true},
		{"gzip", true},
		{"pgzip-best-compression", true},
		{"deflate-default", true},
		{"", false},
		{"zstd-best-compression", false},
		{"lz4", false},
		{"bogus", false},
	}
	for _, tt := range tests {
		t.Run(tt.algorithm, func(t *testing.T) {
			if got := IsValidCompressionAlgorithm(tt.algorithm); got != tt.want {
				t.Errorf("IsValidCompressionAlgorithm(%q) = %v, want %v", tt.algorithm, got, tt.want)
			}
		})
	}
}

func TestCompressionPolicyValidate(t *testing.T) {
	tests := []struct {
		name    string
		policy  *CompressionPolicy
		wantErr bool
	}{
		{"nil policy", nil, false},
		{"empty algorithm", &CompressionPolicy{}, false},
		{"valid algorithm", &CompressionPolicy{Algorithm: "zstd"}, false},
		{"none algorithm", &CompressionPolicy{Algorithm: "none"}, false},
		{"invalid algorithm", &CompressionPolicy{Algorithm: "bogus"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.policy.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, ErrInvalidCompressionAlgorithm) {
				t.Errorf("Validate() error = %v, want it to wrap ErrInvalidCompressionAlgorithm", err)
			}
		})
	}
}

func TestDataValidateCompression(t *testing.T) {
	base := func() Data {
		return Data{
			Client: ClientConfig{
				ClusterName: "cluster",
				Base: BaseRepositoryClientConfig{
					URL:            "https://example",
					ServerCertPath: "s",
					ClientCertPath: "c",
					ClientKeyPath:  "k",
				},
			},
		}
	}

	t.Run("valid tier1 and tier2 compression", func(t *testing.T) {
		d := base()
		d.Tier1CompressionPolicy = &CompressionPolicy{Algorithm: "zstd"}
		d.Tier2CompressionPolicy = &CompressionPolicy{Algorithm: "s2-default"}
		if err := d.Validate(); err != nil {
			t.Errorf("Validate() unexpected error = %v", err)
		}
	})

	t.Run("invalid tier1 compression", func(t *testing.T) {
		d := base()
		d.Tier1CompressionPolicy = &CompressionPolicy{Algorithm: "bogus"}
		if err := d.Validate(); !errors.Is(err, ErrInvalidCompressionAlgorithm) {
			t.Errorf("Validate() error = %v, want ErrInvalidCompressionAlgorithm", err)
		}
	})

	t.Run("invalid tier2 compression", func(t *testing.T) {
		d := base()
		d.Tier2CompressionPolicy = &CompressionPolicy{Algorithm: "bogus"}
		if err := d.Validate(); !errors.Is(err, ErrInvalidCompressionAlgorithm) {
			t.Errorf("Validate() error = %v, want ErrInvalidCompressionAlgorithm", err)
		}
	})
}
