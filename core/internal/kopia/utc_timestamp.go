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
	"encoding/json"
	"fmt"
	"time"
)

// UTCTimestamp stores the UTC timestamp in nanoseconds and provides JSON serializability.
//
//nolint:recvcheck
type UTCTimestamp int64

// UnmarshalJSON implements json.Unmarshaler.
func (u *UTCTimestamp) UnmarshalJSON(v []byte) error {
	var t time.Time

	if err := t.UnmarshalJSON(v); err != nil {
		return fmt.Errorf("unable to unmarshal time: %w", err)
	}

	*u = UTCTimestamp(t.UnixNano())

	return nil
}

// MarshalJSON implements json.Marshaler.
func (u UTCTimestamp) MarshalJSON() ([]byte, error) {
	return u.ToTime().UTC().MarshalJSON()
}

// ToTime returns time.Time representation of the time.
func (u UTCTimestamp) ToTime() time.Time {
	return time.Unix(0, int64(u))
}

// Add adds the specified duration to UTC time.
func (u UTCTimestamp) Add(dur time.Duration) UTCTimestamp {
	return u + UTCTimestamp(dur)
}

// Sub returns the difference between two specified durations.
func (u UTCTimestamp) Sub(u2 UTCTimestamp) time.Duration {
	return time.Duration(u - u2)
}

// After returns true if the timestamp is after another timestamp.
func (u UTCTimestamp) After(other UTCTimestamp) bool {
	return u > other
}

// Before returns true if the timestamp is before another timestamp.
func (u UTCTimestamp) Before(other UTCTimestamp) bool {
	return u < other
}

// Equal returns true if the timestamp is equal to another timestamp.
func (u UTCTimestamp) Equal(other UTCTimestamp) bool {
	return u == other
}

// Format formats the timestamp according to the provided layout.
func (u UTCTimestamp) Format(layout string) string {
	return u.ToTime().UTC().Format(layout)
}

var (
	_ json.Marshaler   = UTCTimestamp(0)
	_ json.Unmarshaler = (*UTCTimestamp)(nil)
)
