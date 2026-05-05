package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// GroupVersion is group version used to register these objects.
	//
	//nolint:gochecknoglobals
	GroupVersion = schema.GroupVersion{Group: "klio.cnpg.io", Version: "v1alpha1"}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme.
	//
	//nolint:gochecknoglobals
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme adds the types in this group-version to the given scheme.
	//
	//nolint:gochecknoglobals
	AddToScheme = SchemeBuilder.AddToScheme
)

func addKnownTypes(s *runtime.Scheme) error {
	s.AddKnownTypes(GroupVersion,
		&PluginConfiguration{},
		&PluginConfigurationList{},
		&Server{},
		&ServerList{},
	)
	metav1.AddToGroupVersion(s, GroupVersion)

	return nil
}
