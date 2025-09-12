// Package v1alpha1 contains API Schema definitions for the klio v1alpha1 API group.
// +kubebuilder:object:generate=true
// +groupName=kliocatalog.cnpg.io
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	// GroupVersion is group version used to register these objects.
	//
	//nolint:gochecknoglobals
	GroupVersion = schema.GroupVersion{Group: "kliocatalog.cnpg.io", Version: "v1alpha1"}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme.
	//
	//nolint:gochecknoglobals
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	//
	//nolint:gochecknoglobals
	AddToScheme = SchemeBuilder.AddToScheme
)
