package features

import (
	"sigs.k8s.io/e2e-framework/pkg/types"
)

// PluginConfigurationUpdateFeature defines a feature for testing PluginConfiguration updates
// and verifying that sidecar containers restart when configuration changes.
type PluginConfigurationUpdateFeature struct {
	name     string
	setup    types.StepFunc
	run      types.StepFunc
	teardown types.StepFunc
}

// PluginConfigurationUpdateFeatureConfig holds the configuration for creating
// a PluginConfiguration update feature test.
type PluginConfigurationUpdateFeatureConfig struct {
	// Name of the feature test.
	Name string
	// Setup function to initialize test resources.
	Setup types.StepFunc
	// Run function to execute the test logic.
	Run types.StepFunc
	// Teardown function to clean up test resources.
	Teardown types.StepFunc
}

// NewPluginConfigurationUpdateFeature creates a new PluginConfigurationUpdateFeature
// with the given configuration.
func NewPluginConfigurationUpdateFeature(
	config PluginConfigurationUpdateFeatureConfig,
) *PluginConfigurationUpdateFeature {
	return &PluginConfigurationUpdateFeature{
		name:     config.Name,
		setup:    config.Setup,
		run:      config.Run,
		teardown: config.Teardown,
	}
}

// Name returns the name of the feature.
func (f *PluginConfigurationUpdateFeature) Name() string {
	return f.name
}

// Setup initializes the feature test, setting up the necessary resources.
func (f *PluginConfigurationUpdateFeature) Setup() types.StepFunc {
	return f.setup
}

// Run executes the feature test.
func (f *PluginConfigurationUpdateFeature) Run() types.StepFunc {
	return f.run
}

// Teardown cleans up resources after the test is run.
func (f *PluginConfigurationUpdateFeature) Teardown() types.StepFunc {
	return f.teardown
}
