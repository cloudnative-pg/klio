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

package kopia

import (
	"testing"
	"time"
)

func TestUTCTimestamp(t *testing.T) {
	now := time.Now().UTC()
	ts := UTCTimestamp(now.UnixNano())

	// Test ToTime
	if !ts.ToTime().Equal(now) {
		t.Errorf("ToTime() = %v, want %v", ts.ToTime(), now)
	}

	// Test arithmetic (Add/Sub)
	duration := 5 * time.Minute
	later := ts.Add(duration)
	if later.Sub(ts) != duration {
		t.Errorf("Sub() difference = %v, want %v", later.Sub(ts), duration)
	}

	// Test comparisons
	if !later.After(ts) {
		t.Error("After() should be true for later timestamp")
	}
	if !ts.Before(later) {
		t.Error("Before() should be true for earlier timestamp")
	}
	if !ts.Equal(UTCTimestamp(now.UnixNano())) {
		t.Error("Equal() should be true for same timestamp")
	}

	// Test formatting
	expectedFormat := now.Format(time.RFC3339)
	if got := ts.Format(time.RFC3339); got != expectedFormat {
		t.Errorf("Format() = %q, want %q", got, expectedFormat)
	}
}
