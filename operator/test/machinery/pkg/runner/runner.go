package runner

import (
	"context"
	"log"
	"os"
	"testing"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	e2eFrameworkFeatures "sigs.k8s.io/e2e-framework/pkg/features"
	"sigs.k8s.io/e2e-framework/pkg/types"

	machineryFeatures "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/features"
)

var (
	testEnv            env.Environment  //nolint:gochecknoglobals
	registeredFeatures []*FeatureConfig //nolint:gochecknoglobals
	additionalSetup    types.EnvFunc    //nolint:gochecknoglobals
	additionalTeardown types.EnvFunc    //nolint:gochecknoglobals
)

// RegisterFeature registers a single feature with optional configuration.
// By default, features run in parallel. Use WithSerialExecution() for serial execution.
//
// Examples:
//
//	RegisterFeature(BackupFromPrimary(ns))                              // Parallel (default)
//	RegisterFeature(Tier2Retention(ns), WithSerialExecution())          // Serial
func RegisterFeature(f machineryFeatures.Feature, opts ...FeatureOption) {
	cfg := newFeatureConfig(f, opts...)
	registeredFeatures = append(registeredFeatures, cfg)
}

// RegisterFeatures registers multiple features with the same configuration.
//
// Examples:
//
//	RegisterFeatures(nil, feat1, feat2, feat3)                              // All parallel
//	RegisterFeatures([]FeatureOption{WithSerialExecution()}, feat1, feat2)  // All serial
func RegisterFeatures(opts []FeatureOption, features ...machineryFeatures.Feature) {
	for _, f := range features {
		RegisterFeature(f, opts...)
	}
}

// RegisterSetup registers a setup function to be executed before all features run.
func RegisterSetup(setupFunc types.EnvFunc) {
	additionalSetup = setupFunc
}

// RegisterTeardown registers a teardown function to be executed after all features have run.
func RegisterTeardown(teardownFunc types.EnvFunc) {
	additionalTeardown = teardownFunc
}

// RunAllFeatures runs all registered features in two phases:
// 1. Parallel phase: All features marked ExecutionModeParallel run concurrently.
// 2. Serial phase: All features marked ExecutionModeSerial run sequentially.
func RunAllFeatures(t *testing.T) {
	t.Helper()

	// Separate features by execution mode
	var parallelFeatures []*FeatureConfig
	var serialFeatures []*FeatureConfig

	for _, cfg := range registeredFeatures {
		switch cfg.ExecutionMode {
		case ExecutionModeParallel:
			parallelFeatures = append(parallelFeatures, cfg)
		case ExecutionModeSerial:
			serialFeatures = append(serialFeatures, cfg)
		}
	}

	// Phase 1: Run parallel features concurrently
	if len(parallelFeatures) > 0 {
		t.Run("Parallel Features", func(t *testing.T) {
			for _, cfg := range parallelFeatures {
				featureCfg := cfg // Capture for closure

				t.Run(featureCfg.Feature.Name(), func(t *testing.T) {
					t.Parallel() // Enable parallel execution

					f := e2eFrameworkFeatures.New(featureCfg.Feature.Name()).
						Setup(featureCfg.Feature.Setup()).
						Assess(featureCfg.Feature.Name(), featureCfg.Feature.Run()).
						Teardown(featureCfg.Feature.Teardown()).
						Feature()

					testEnv.Test(t, f)
				})
			}
		})
	}

	// Phase 2: Run serial features sequentially
	// Each runs in its own t.Run() WITHOUT t.Parallel()
	for _, cfg := range serialFeatures {
		featureCfg := cfg // Capture for closure

		t.Run(featureCfg.Feature.Name(), func(t *testing.T) {
			// NO t.Parallel() - runs sequentially

			f := e2eFrameworkFeatures.New(featureCfg.Feature.Name()).
				Setup(featureCfg.Feature.Setup()).
				Assess(featureCfg.Feature.Name(), featureCfg.Feature.Run()).
				Teardown(featureCfg.Feature.Teardown()).
				Feature()

			testEnv.Test(t, f)
		})
	}
}

// RunMain initializes the test environment.
func RunMain(m *testing.M) {
	cfg, err := envconf.NewFromFlags()
	if err != nil {
		log.Fatalf("failed to build envconf from flags: %s", err)
	}
	testEnv = env.NewWithConfig(cfg)

	testEnv.Setup(
		func(ctx context.Context, config *envconf.Config) (context.Context, error) {
			if err := cnpgv1.AddToScheme(config.Client().Resources().GetScheme()); err != nil {
				return ctx, err
			}

			return ctx, err
		},
	)

	if additionalSetup != nil {
		testEnv.Setup(additionalSetup)
	}

	if additionalTeardown != nil {
		testEnv.Finish(additionalTeardown)
	}

	os.Exit(testEnv.Run(m))
}
