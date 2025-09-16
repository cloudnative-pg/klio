package podtemplate

import (
	"encoding/json"

	"github.com/cloudnative-pg/machinery/pkg/stringset"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
)

// Merge merges two Pod templates allowing the user to override
// the resource we generate.
// This approach is ignobly stolen from the Prometheus operator.
func Merge(base, overlay *corev1.PodTemplateSpec) (*corev1.PodTemplateSpec, error) {
	// Marshal to JSON, merge, then unmarshal back to struct.
	// This leverages the strategic merge patch logic built into Kubernetes.
	baseJSON, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}

	// Work on a deep copy of the overlay to avoid mutating the caller’s object.
	overlayAug := overlay.DeepCopy()

	// Ensure that every container present in base is present in the overlay too.
	// This is important to avoid deleting containers
	baseContainers := stringset.New()
	for _, container := range base.Spec.Containers {
		baseContainers.Put(container.Name)
	}

	overlayContainers := stringset.New()
	for _, container := range overlayAug.Spec.Containers {
		overlayContainers.Put(container.Name)
	}

	for _, missingContainerName := range baseContainers.Subtract(overlayContainers).ToList() {
		overlayAug.Spec.Containers = append(overlayAug.Spec.Containers, corev1.Container{Name: missingContainerName})
	}

	// Apply the same safety for initContainers so critical init doesn’t get dropped inadvertently.
	baseInitContainers := stringset.New()
	for _, c := range base.Spec.InitContainers {
		baseInitContainers.Put(c.Name)
	}
	overlayInitContainers := stringset.New()
	for _, c := range overlayAug.Spec.InitContainers {
		overlayInitContainers.Put(c.Name)
	}
	for _, missingInitName := range baseInitContainers.Subtract(overlayInitContainers).ToList() {
		overlayAug.Spec.InitContainers = append(overlayAug.Spec.InitContainers, corev1.Container{Name: missingInitName})
	}

	// Proceed with strategic merging
	overlayJSON, err := json.Marshal(overlayAug)
	if err != nil {
		return nil, err
	}

	merged, err := strategicpatch.StrategicMergePatch(baseJSON, overlayJSON, &corev1.PodTemplateSpec{})
	if err != nil {
		return nil, err
	}

	var result corev1.PodTemplateSpec
	err = json.Unmarshal(merged, &result)

	return &result, err
}
