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

package features

import (
	"sigs.k8s.io/e2e-framework/pkg/types"
)

// SimpleFeature is a generic feature implementation for tests that only need
// setup, run, and teardown step functions with no extra fields.
type SimpleFeature struct {
	name     string
	setup    types.StepFunc
	run      types.StepFunc
	teardown types.StepFunc
}

// SimpleFeatureConfig holds the configuration for creating a SimpleFeature.
type SimpleFeatureConfig struct {
	// Name of the feature test.
	Name string
	// Setup function to initialize test resources.
	Setup types.StepFunc
	// Run function to execute the test logic.
	Run types.StepFunc
	// Teardown function to clean up test resources.
	Teardown types.StepFunc
}

// NewSimpleFeature creates a new SimpleFeature with the given configuration.
func NewSimpleFeature(config SimpleFeatureConfig) *SimpleFeature {
	return &SimpleFeature{
		name:     config.Name,
		setup:    config.Setup,
		run:      config.Run,
		teardown: config.Teardown,
	}
}

// Name returns the name of the feature.
func (f *SimpleFeature) Name() string {
	return f.name
}

// Setup initializes the feature test, setting up the necessary resources.
func (f *SimpleFeature) Setup() types.StepFunc {
	return f.setup
}

// Run executes the feature test.
func (f *SimpleFeature) Run() types.StepFunc {
	return f.run
}

// Teardown cleans up resources after the test is run.
func (f *SimpleFeature) Teardown() types.StepFunc {
	return f.teardown
}
