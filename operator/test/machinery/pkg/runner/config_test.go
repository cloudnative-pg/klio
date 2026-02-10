package runner

import (
	"context"
	"testing"

	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/types"
)

// mockFeature implements the Feature interface for testing.
type mockFeature struct {
	name string
}

func (m *mockFeature) Name() string {
	return m.name
}

func (m *mockFeature) Setup() types.StepFunc {
	return func(ctx context.Context, _ *testing.T, _ *envconf.Config) context.Context {
		return ctx
	}
}

func (m *mockFeature) Run() types.StepFunc {
	return func(ctx context.Context, _ *testing.T, _ *envconf.Config) context.Context {
		return ctx
	}
}

func (m *mockFeature) Teardown() types.StepFunc {
	return func(ctx context.Context, _ *testing.T, _ *envconf.Config) context.Context {
		return ctx
	}
}

func TestNewFeatureConfig(t *testing.T) {
	feat := &mockFeature{name: "test-feature"}

	t.Run("default is parallel", func(t *testing.T) {
		cfg := newFeatureConfig(feat)
		if cfg.ExecutionMode != ExecutionModeParallel {
			t.Errorf("expected ExecutionModeParallel, got %v", cfg.ExecutionMode)
		}
		if cfg.Feature != feat {
			t.Errorf("expected feature to be set")
		}
	})

	t.Run("WithSerialExecution sets serial mode", func(t *testing.T) {
		cfg := newFeatureConfig(feat, WithSerialExecution())
		if cfg.ExecutionMode != ExecutionModeSerial {
			t.Errorf("expected ExecutionModeSerial, got %v", cfg.ExecutionMode)
		}
	})

	t.Run("WithParallelExecution sets parallel mode explicitly", func(t *testing.T) {
		cfg := newFeatureConfig(feat, WithParallelExecution())
		if cfg.ExecutionMode != ExecutionModeParallel {
			t.Errorf("expected ExecutionModeParallel, got %v", cfg.ExecutionMode)
		}
	})

	t.Run("multiple options apply in order", func(t *testing.T) {
		cfg := newFeatureConfig(feat, WithSerialExecution(), WithParallelExecution())
		if cfg.ExecutionMode != ExecutionModeParallel {
			t.Errorf("expected ExecutionModeParallel (last option wins), got %v", cfg.ExecutionMode)
		}
	})
}

func TestRegisterFeature(t *testing.T) {
	// Save and restore original state
	originalFeatures := registeredFeatures
	defer func() {
		registeredFeatures = originalFeatures
	}()

	// Reset for test
	registeredFeatures = nil

	feat1 := &mockFeature{name: "feature1"}
	feat2 := &mockFeature{name: "feature2"}

	RegisterFeature(feat1)
	RegisterFeature(feat2, WithSerialExecution())

	if len(registeredFeatures) != 2 {
		t.Fatalf("expected 2 registered features, got %d", len(registeredFeatures))
	}

	if registeredFeatures[0].ExecutionMode != ExecutionModeParallel {
		t.Errorf("expected feature1 to be parallel")
	}

	if registeredFeatures[1].ExecutionMode != ExecutionModeSerial {
		t.Errorf("expected feature2 to be serial")
	}
}

func TestRegisterFeatures(t *testing.T) {
	// Save and restore original state
	originalFeatures := registeredFeatures
	defer func() {
		registeredFeatures = originalFeatures
	}()

	// Reset for test
	registeredFeatures = nil

	feat1 := &mockFeature{name: "feature1"}
	feat2 := &mockFeature{name: "feature2"}
	feat3 := &mockFeature{name: "feature3"}

	// Register multiple with serial execution
	RegisterFeatures([]FeatureOption{WithSerialExecution()}, feat1, feat2)

	// Register multiple with default (parallel)
	RegisterFeatures(nil, feat3)

	if len(registeredFeatures) != 3 {
		t.Fatalf("expected 3 registered features, got %d", len(registeredFeatures))
	}

	if registeredFeatures[0].ExecutionMode != ExecutionModeSerial {
		t.Errorf("expected feature1 to be serial")
	}

	if registeredFeatures[1].ExecutionMode != ExecutionModeSerial {
		t.Errorf("expected feature2 to be serial")
	}

	if registeredFeatures[2].ExecutionMode != ExecutionModeParallel {
		t.Errorf("expected feature3 to be parallel")
	}
}
