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

// RetentionPolicy defines how many backups we should keep.
type RetentionPolicy struct {
	// KeepLatest is the number of latest backups to keep
	KeepLatest *int `json:"keep_latest,omitempty" mapstructure:"keep_latest"`

	// KeepAnnual is the number of annual backups to keep
	KeepAnnual *int `json:"keep_annual,omitempty" mapstructure:"keep_annual"`

	// KeepMonthly is the number of monthly backups to keep
	KeepMonthly *int `json:"keep_monthly,omitempty" mapstructure:"keep_monthly"`

	// KeepWeekly is the number of weekly backups to keep
	KeepWeekly *int `json:"keep_weekly,omitempty" mapstructure:"keep_weekly"`

	// KeepDaily is the number of daily backups to keep
	KeepDaily *int `json:"keep_daily,omitempty" mapstructure:"keep_daily"`

	// KeepHourly is the number of hourly backups to keep
	KeepHourly *int `json:"keep_hourly,omitempty" mapstructure:"keep_hourly"`
}
