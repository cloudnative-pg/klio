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

package cnpgi

import (
	"fmt"

	"github.com/spf13/afero"
	"sigs.k8s.io/yaml"

	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// Retention contains the retention policy configuration.
type Retention struct {
	KeepLatest  *int
	KeepAnnual  *int
	KeepMonthly *int
	KeepWeekly  *int
	KeepDaily   *int
	KeepHourly  *int
}

// IsEmpty checks if the retention configuration is empty or not.
func (r *Retention) IsEmpty() bool {
	if r == nil {
		return true
	}

	emptyRetention := Retention{}

	return *r == emptyRetention
}

// extractTier1RetentionFromConfiguration reads retention policy settings.
func extractTier1RetentionFromConfiguration() (*Retention, error) {
	osFS := afero.NewOsFs()
	f, err := afero.ReadFile(osFS, backupRepositoryConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read backup repository config file %q: %w", backupRepositoryConfigPath, err)
	}

	var configData config.Data
	err = yaml.Unmarshal(f, &configData)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal backup repository config file %q: %w", backupRepositoryConfigPath, err)
	}

	conf := Retention{}

	if configData.Tier1RetentionPolicy == nil {
		return &conf, nil
	}

	conf.KeepLatest = configData.Tier1RetentionPolicy.KeepLatest
	conf.KeepAnnual = configData.Tier1RetentionPolicy.KeepAnnual
	conf.KeepMonthly = configData.Tier1RetentionPolicy.KeepMonthly
	conf.KeepWeekly = configData.Tier1RetentionPolicy.KeepWeekly
	conf.KeepDaily = configData.Tier1RetentionPolicy.KeepDaily
	conf.KeepHourly = configData.Tier1RetentionPolicy.KeepHourly

	return &conf, nil
}
