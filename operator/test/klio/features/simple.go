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
