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
		wantErr error
	}{
		{"nil policy", nil, nil},
		{"empty algorithm", &CompressionPolicy{}, nil},
		{"valid algorithm", &CompressionPolicy{Algorithm: "zstd"}, nil},
		{"none algorithm", &CompressionPolicy{Algorithm: "none"}, nil},
		{"invalid algorithm", &CompressionPolicy{Algorithm: "bogus"}, ErrInvalidCompressionAlgorithm},
		{
			"min below max",
			&CompressionPolicy{Algorithm: "zstd", MinSize: 4096, MaxSize: 1048576},
			nil,
		},
		{
			"min equal to max compresses only that size",
			&CompressionPolicy{Algorithm: "zstd", MinSize: 4096, MaxSize: 4096},
			nil,
		},
		{
			"min without max",
			&CompressionPolicy{Algorithm: "zstd", MinSize: 4096},
			nil,
		},
		{
			"max without min",
			&CompressionPolicy{Algorithm: "zstd", MaxSize: 4096},
			nil,
		},
		{
			"min above max excludes every file",
			&CompressionPolicy{Algorithm: "zstd", MinSize: 1048576, MaxSize: 4096},
			ErrInvalidCompressionSizeRange,
		},
		{
			// The size bounds are emitted independently of the algorithm, so
			// an inconsistent range must be rejected even without one.
			"min above max without an algorithm",
			&CompressionPolicy{MinSize: 1048576, MaxSize: 4096},
			ErrInvalidCompressionSizeRange,
		},
		{
			// An invalid algorithm is reported even when the range is fine.
			"invalid algorithm with a valid range",
			&CompressionPolicy{Algorithm: "bogus", MinSize: 1, MaxSize: 2},
			ErrInvalidCompressionAlgorithm,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.policy.Validate()
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}

				return
			}

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Validate() error = %v, want it to wrap %v", err, tt.wantErr)
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

	t.Run("inconsistent tier1 compression size range", func(t *testing.T) {
		d := base()
		d.Tier1CompressionPolicy = &CompressionPolicy{
			Algorithm: "zstd", MinSize: 1048576, MaxSize: 4096,
		}
		if err := d.Validate(); !errors.Is(err, ErrInvalidCompressionSizeRange) {
			t.Errorf("Validate() error = %v, want ErrInvalidCompressionSizeRange", err)
		}
	})

	t.Run("inconsistent tier2 compression size range", func(t *testing.T) {
		d := base()
		d.Tier2CompressionPolicy = &CompressionPolicy{
			Algorithm: "zstd", MinSize: 1048576, MaxSize: 4096,
		}
		if err := d.Validate(); !errors.Is(err, ErrInvalidCompressionSizeRange) {
			t.Errorf("Validate() error = %v, want ErrInvalidCompressionSizeRange", err)
		}
	})
}
