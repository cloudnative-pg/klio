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
	"encoding/json"
	"errors"
	"fmt"
)

// RetentionPolicy defines which backups Klio should keep. It is evaluated by
// the server against the backup catalog, replacing the retention that Kopia
// used to enforce on its own.
type RetentionPolicy struct {
	// Latest keeps only the given number of most recent backups and deletes the
	// rest. A configured policy always sets it to at least 1; keeping every
	// backup is expressed by not configuring a retention policy at all.
	Latest int `json:"latest,omitempty" mapstructure:"latest"`
}

// Validate implements a custom validation function for RetentionPolicy.
func (r *RetentionPolicy) Validate() error {
	if r.Latest < 1 {
		return errors.New("invalid retention policy: latest must be greater than or equal to 1")
	}

	return nil
}

// MarshalWire serializes the policy to the JSON string used to carry it to the
// server. A nil policy yields an empty string, which the server reads as "keep
// everything".
func (r *RetentionPolicy) MarshalWire() (string, error) {
	if r == nil {
		return "", nil
	}

	content, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("while serializing retention policy: %w", err)
	}

	return string(content), nil
}
