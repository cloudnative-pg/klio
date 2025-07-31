package features

import (
	"sigs.k8s.io/e2e-framework/pkg/types"
)

// Feature defines the interface for a feature in the test environment.
type Feature interface {
	Name() string
	Setup() types.StepFunc
	Run() types.StepFunc
	Teardown() types.StepFunc
}
