package runner

import (
	machineryFeatures "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/features"
)

// ExecutionMode defines how a feature should be executed.
type ExecutionMode int

const (
	// ExecutionModeParallel runs the feature concurrently with other parallel features.
	ExecutionModeParallel ExecutionMode = iota

	// ExecutionModeSerial runs the feature sequentially after all parallel features complete.
	ExecutionModeSerial
)

// FeatureConfig wraps a feature with execution configuration.
type FeatureConfig struct {
	Feature       machineryFeatures.Feature
	ExecutionMode ExecutionMode
}

// FeatureOption configures a FeatureConfig.
type FeatureOption func(*FeatureConfig)

// WithSerialExecution marks a feature to run serially after all parallel features.
func WithSerialExecution() FeatureOption {
	return func(cfg *FeatureConfig) {
		cfg.ExecutionMode = ExecutionModeSerial
	}
}

// WithParallelExecution explicitly marks a feature to run in parallel (default).
func WithParallelExecution() FeatureOption {
	return func(cfg *FeatureConfig) {
		cfg.ExecutionMode = ExecutionModeParallel
	}
}

// newFeatureConfig creates a FeatureConfig with defaults.
func newFeatureConfig(feature machineryFeatures.Feature, opts ...FeatureOption) *FeatureConfig {
	cfg := &FeatureConfig{
		Feature:       feature,
		ExecutionMode: ExecutionModeParallel, // Default: parallel
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return cfg
}
