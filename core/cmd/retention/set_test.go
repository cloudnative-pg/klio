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

package retention

import (
	"testing"

	"github.com/cloudnative-pg/klio/core/pkg/config"
)

func TestToGRPCRetentionPolicyNoPolicy(t *testing.T) {
	got := toGRPCRetentionPolicy(config.Data{})

	if got.GetTier1Policy() != nil || got.GetTier2Policy() != nil {
		t.Errorf("expected no tier policy, got %v", got)
	}
}

// An empty retention block is valid in the CRD and must keep every backup
// instead of dereferencing a nil latest.
func TestToGRPCRetentionPolicyEmptyBlock(t *testing.T) {
	got := toGRPCRetentionPolicy(config.Data{
		Tier1RetentionPolicy: &config.RetentionPolicy{},
		Tier2RetentionPolicy: &config.RetentionPolicy{},
	})

	if got.GetTier1Policy() == nil || got.GetTier1Policy().GetLatest() != 0 {
		t.Errorf("expected a tier1 policy with latest 0, got %v", got.GetTier1Policy())
	}

	if got.GetTier2Policy() == nil || got.GetTier2Policy().GetLatest() != 0 {
		t.Errorf("expected a tier2 policy with latest 0, got %v", got.GetTier2Policy())
	}
}

func TestToGRPCRetentionPolicyLatest(t *testing.T) {
	got := toGRPCRetentionPolicy(config.Data{
		Tier1RetentionPolicy: &config.RetentionPolicy{Latest: new(int32(5))},
		Tier2RetentionPolicy: &config.RetentionPolicy{Latest: new(int32(10))},
	})

	if got.GetTier1Policy().GetLatest() != 5 {
		t.Errorf("expected tier1 latest 5, got %d", got.GetTier1Policy().GetLatest())
	}

	if got.GetTier2Policy().GetLatest() != 10 {
		t.Errorf("expected tier2 latest 10, got %d", got.GetTier2Policy().GetLatest())
	}
}

func TestToGRPCRetentionPolicyOnlyConfiguredTier(t *testing.T) {
	got := toGRPCRetentionPolicy(config.Data{
		Tier1RetentionPolicy: &config.RetentionPolicy{Latest: new(int32(3))},
	})

	if got.GetTier1Policy() == nil {
		t.Error("expected a tier1 policy")
	}

	if got.GetTier2Policy() != nil {
		t.Errorf("expected no tier2 policy, got %v", got.GetTier2Policy())
	}
}
