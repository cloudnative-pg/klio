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
	testEnv            env.Environment             //nolint:gochecknoglobals
	registeredFeatures []machineryFeatures.Feature //nolint:gochecknoglobals
	additionalSetup    types.EnvFunc               //nolint:gochecknoglobals
	additionalTeardown types.EnvFunc               //nolint:gochecknoglobals
)

// RegisterFeature registers a single feature to be run in the test environment.
func RegisterFeature(f machineryFeatures.Feature) {
	registeredFeatures = append(registeredFeatures, f)
}

// RegisterFeatures registers multiple features to be run in the test environment.
func RegisterFeatures(features ...machineryFeatures.Feature) {
	registeredFeatures = append(registeredFeatures, features...)
}

// RegisterSetup registers a setup function to be executed before all features run.
func RegisterSetup(setupFunc types.EnvFunc) {
	additionalSetup = setupFunc
}

// RegisterTeardown registers a teardown function to be executed after all features have run.
func RegisterTeardown(teardownFunc types.EnvFunc) {
	additionalTeardown = teardownFunc
}

// RunAllFeatures runs all registered features in the test environment.
func RunAllFeatures(t *testing.T) {
	t.Helper()
	for _, registeredFeature := range registeredFeatures {
		f := e2eFrameworkFeatures.New(registeredFeature.Name()).
			Setup(registeredFeature.Setup()).
			Assess(registeredFeature.Name(), registeredFeature.Run()).
			Teardown(registeredFeature.Teardown()).
			Feature()

		testEnv.Test(t, f)
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
