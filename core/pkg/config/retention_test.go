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

import "testing"

func TestRetentionPolicyValidate(t *testing.T) {
	tests := []struct {
		name    string
		policy  RetentionPolicy
		wantErr bool
	}{
		{name: "latest 1 is valid", policy: RetentionPolicy{Latest: 1}},
		{name: "latest 10 is valid", policy: RetentionPolicy{Latest: 10}},
		{name: "latest 0 is invalid", policy: RetentionPolicy{Latest: 0}, wantErr: true},
		{name: "negative latest is invalid", policy: RetentionPolicy{Latest: -1}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.policy.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestRetentionPolicyMarshalWire(t *testing.T) {
	t.Run("nil policy yields empty string", func(t *testing.T) {
		var p *RetentionPolicy
		got, err := p.MarshalWire()
		if err != nil || got != "" {
			t.Fatalf("MarshalWire() = %q, %v; want \"\", nil", got, err)
		}
	})

	t.Run("configured policy serializes latest", func(t *testing.T) {
		got, err := (&RetentionPolicy{Latest: 3}).MarshalWire()
		if err != nil || got != `{"latest":3}` {
			t.Fatalf("MarshalWire() = %q, %v; want %q, nil", got, err, `{"latest":3}`)
		}
	})
}
