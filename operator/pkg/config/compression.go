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
	"fmt"
)

// CompressionPolicy configures the Kopia compression policy applied to base
// backup data.
type CompressionPolicy struct {
	// Algorithm is the name of the Kopia compression algorithm to use.
	// The special value "none" disables compression.
	Algorithm string `json:"algorithm,omitempty" mapstructure:"algorithm"`

	// MinSize is the minimum file size, in bytes, to attempt compression for.
	// Files smaller than this are stored uncompressed. Zero means no minimum.
	MinSize int64 `json:"min_size,omitempty" mapstructure:"min_size"`

	// MaxSize is the maximum file size, in bytes, to attempt compression for.
	// Files larger than this are stored uncompressed. Zero means no maximum.
	MaxSize int64 `json:"max_size,omitempty" mapstructure:"max_size"`
}

// ErrInvalidCompressionAlgorithm is returned when an unsupported compression
// algorithm is configured.
var ErrInvalidCompressionAlgorithm = errors.New("invalid compression algorithm")

// IsValidCompressionAlgorithm returns true when the passed algorithm is a
// supported Kopia compression algorithm. The list matches the compressors
// registered by Kopia, plus the special value "none" that explicitly disables
// compression. It must be kept in sync with the CompressionAlgorithm enum in
// the operator CRD types.
func IsValidCompressionAlgorithm(algorithm string) bool {
	switch algorithm {
	case "none",
		"deflate-best-compression",
		"deflate-best-speed",
		"deflate-default",
		"gzip",
		"gzip-best-compression",
		"gzip-best-speed",
		"pgzip",
		"pgzip-best-compression",
		"pgzip-best-speed",
		"s2-better",
		"s2-default",
		"s2-parallel-4",
		"s2-parallel-8",
		"zstd",
		"zstd-better-compression",
		"zstd-fastest":
		return true
	default:
		return false
	}
}

// Validate checks that the configured compression algorithm is supported.
// A nil policy or an empty algorithm is considered valid and means the Kopia
// default (inherited) policy is left untouched.
func (c *CompressionPolicy) Validate() error {
	if c == nil || c.Algorithm == "" {
		return nil
	}

	if !IsValidCompressionAlgorithm(c.Algorithm) {
		return fmt.Errorf("%w: %q", ErrInvalidCompressionAlgorithm, c.Algorithm)
	}

	return nil
}
