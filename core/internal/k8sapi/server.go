package k8sapi

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiextensions-apiserver/pkg/apiserver"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/apiserver/pkg/endpoints/openapi"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/server/options"
	"k8s.io/apiserver/pkg/util/compatibility"
	basecompatibility "k8s.io/component-base/compatibility"
	"k8s.io/component-base/featuregate"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/kopia"
	"github.com/cloudnative-pg/klio/core/internal/k8sapi/v1alpha1"
)

const (
	apiServerName        = "klio-apiserver"
	klioAPIServerVersion = "0.0.8" // x-release-please-version
)

// Start starts an API server.
func Start(
	ctx context.Context,
	connection *kopia.Connection,
	certFile, keyFile string,
) error {
	scheme := runtime.NewScheme()
	codecs := serializer.NewCodecFactory(scheme)

	// Set up API server config
	opts := options.NewRecommendedOptions("", nil)

	// If a certificate and key are provided, use them for the API server
	if certFile != "" || keyFile != "" {
		if certFile == "" || keyFile == "" {
			return fmt.Errorf("both APIServerCertFile and APIServerKeyFile must be provided together, "+
				"got cert=%q key=%q", certFile, keyFile)
		}
		opts.SecureServing.ServerCert.CertKey.CertFile = certFile
		opts.SecureServing.ServerCert.CertKey.KeyFile = keyFile
	}

	genericConfig := genericapiserver.NewRecommendedConfig(codecs)
	if err := opts.ApplyTo(genericConfig); err != nil {
		return err
	}

	_ = compatibility.DefaultComponentGlobalsRegistry.Register(
		apiServerName,
		basecompatibility.NewEffectiveVersionFromString(klioAPIServerVersion, "", ""),
		featuregate.NewVersionedFeatureGate(version.MustParse(klioAPIServerVersion)),
	)

	genericConfig.OpenAPIConfig = genericapiserver.DefaultOpenAPIConfig(
		v1alpha1.GetOpenAPIDefinitions,
		openapi.NewDefinitionNamer(apiserver.Scheme),
	)
	genericConfig.OpenAPIConfig.Info.Title = "Klio"
	genericConfig.OpenAPIConfig.Info.Version = klioAPIServerVersion

	genericConfig.OpenAPIV3Config = genericapiserver.DefaultOpenAPIV3Config(
		v1alpha1.GetOpenAPIDefinitions,
		openapi.NewDefinitionNamer(apiserver.Scheme),
	)
	genericConfig.OpenAPIV3Config.Info.Title = "Klio"
	genericConfig.OpenAPIV3Config.Info.Version = klioAPIServerVersion

	genericConfig.EffectiveVersion = compatibility.DefaultComponentGlobalsRegistry.EffectiveVersionFor(
		apiServerName)
	genericConfig.FeatureGate = compatibility.DefaultComponentGlobalsRegistry.FeatureGateFor(
		basecompatibility.DefaultKubeComponent)

	apiServer, err := genericConfig.Complete().New(apiServerName, genericapiserver.NewEmptyDelegate())
	if err != nil {
		return err
	}

	// Register API group
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(
		v1alpha1.GroupVersion.Group,
		scheme,
		metav1.ParameterCodec,
		codecs,
	)
	apiGroupInfo.VersionedResourcesStorageMap[v1alpha1.GroupVersion.Version] = map[string]rest.Storage{
		"kliobackups": NewREST(connection),
	}

	if err := apiServer.InstallAPIGroup(&apiGroupInfo); err != nil {
		return err
	}

	if err := apiServer.PrepareRun().RunWithContext(ctx); err != nil {
		return err
	}

	return nil
}
