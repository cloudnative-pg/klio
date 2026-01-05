// A generated module for Devenv functions
//
// This module has been generated via dagger init and serves as a reference to
// basic module structure as you get started with Dagger.
//
// Two functions have been pre-created. You can modify, delete, or add to them,
// as needed. They demonstrate usage of arguments and return types using simple
// echo and grep commands. The functions can be called from the dagger CLI or
// from one of the SDKs.
//
// The first line in this comment block is a short description line and the
// rest is a long description with more detail on the module's purpose or usage,
// if appropriate. All modules should have a short description.

package main

import (
	"context"
	"time"

	"dagger/devenv/internal/dagger"
)

type Devenv struct{}

const goVersion = "1.24.4"

// Build compiles and build the binary
func (m *Devenv) buildCore(
	source *dagger.Directory,
) *dagger.File {
	return dag.Go(dagger.GoOpts{Version: goVersion}).
		WithEnvVariable("GOEXPERIMENT", "boringcrypto").
		Build(source, dagger.GoBuildOpts{
			Tags: []string{"viper_bind_struct"},
		})
}

func (m *Devenv) Kubernetes(
	ctx context.Context,
	source *dagger.Directory,
// +optional
// renovate image: datasource=docker depName=registry versioning=docker
// +default="registry:3.0@sha256:cd92709b4191c5779cd7215ccd695db6c54652e7a62843197e367427efb84d0e"
	registryImage string,
// +optional
// renovate image: datasource=docker depName=skopeo lookupName=quay.io/skopeo/stable versioning=docker
// +default="quay.io/skopeo/stable:v1.21.0@sha256:0f49cd40bcccab07fe26e2d68db4aeb034c10782d60d095f6da0d2267c4ec6ab"
	skopeoImage string,
// renovate image: datasource=docker depName=k3s lookupName=rancher/k3s versioning=docker
// +default="rancher/k3s:v1.35.0-k3s1@sha256:10464930d9bad0c06aef9830e84cd4019c24ed44d5eab594efb7416119097248"
	k3SImage string,
// renovate image: datasource=docker depName=alpine/k8s versioning=docker
// +default="alpine/k8s:1.35.0@sha256:b01ed7ee5807e1abce433fba29447595b6157851054a649c2aafd6c22a3aa16c
	alpineK8S string,
) (*dagger.Container, error) {
	klioImage := source.
		Directory("core").
		WithFile("dist/klio_linux_amd64/klio", m.buildCore(source.Directory("core"))).
		DockerBuild(dagger.DirectoryDockerBuildOpts{
			BuildArgs: []dagger.BuildArg{
				{
					Name:  "TARGETOS",
					Value: "linux",
				},
				{
					Name:  "TARGETARCH",
					Value: "amd64",
				},
			},
		})

	operatorImage := source.
		Directory("operator").
		DockerBuild()

	registrySvc := dag.Container().
		From(registryImage).
		WithExposedPort(5000).
		AsService()

	_, err := dag.Container().From(skopeoImage).
		WithServiceBinding("registry.dev", registrySvc).
		WithEnvVariable("BUST", time.Now().String()).
		WithWorkdir("/tmp").
		WithFile("/tmp/klio_tarball.tar", klioImage.AsTarball()).
		WithFile("/tmp/klio_op_tarball.tar", operatorImage.AsTarball()).
		WithExec(
			[]string{
				"copy",
				"--dest-tls-verify=false",
				"docker-archive:klio_tarball.tar",
				"docker://registry.dev:5000/klio-testing:dev",
			}, dagger.ContainerWithExecOpts{UseEntrypoint: true},
		).
		WithExec(
			[]string{
				"copy",
				"--dest-tls-verify=false",
				"docker-archive:klio_op_tarball.tar",
				"docker://registry.dev:5000/klio-operator-testing:dev",
			}, dagger.ContainerWithExecOpts{UseEntrypoint: true},
		).Sync(ctx)

	if err != nil {
		return nil, err
	}

	k3s := dag.K3S("k3s-test", dagger.K3SOpts{
		Image: k3SImage,
	})
	_, err = k3s.
		With(func(k *dagger.K3S) *dagger.K3S {
			return k.WithContainer(
				k.Container().
					WithEnvVariable("BUST", time.Now().String()).
					WithExec([]string{"sh", "-c", `
cat <<EOF > /etc/rancher/k3s/registries.yaml
mirrors:
  "registry.dev:5000":
    endpoint:
      - "http://registry.dev:5000"
EOF`}).
					WithServiceBinding("registry.dev", registrySvc),
			)
		}).Server().Start(ctx)
	if err != nil {
		return nil, err
	}

	kubectlCtr := dag.Container().From(alpineK8S).
		WithExec([]string{"apk", "add", "--no-cache", "k9s"}).
		WithEnvVariable("KUBECONFIG", "/.kube/config").
		WithFile("/.kube/config", k3s.Config()).
		WithDirectory("/operator", source.Directory("operator")).
		WithExec([]string{"kubectl", "apply", "--server-side", "-f",
			"https://raw.githubusercontent.com/cloudnative-pg/artifacts/refs/heads/main/manifests/operator-manifest.yaml"}).
		WithExec([]string{"kubectl", "rollout", "status", "deployment", "cnpg-controller-manager", "-n",
			"cnpg-system"}).
		WithExec([]string{"kubectl", "apply", "-f",
			"https://github.com/cert-manager/cert-manager/releases/download/v1.18.0/cert-manager.yaml"}).
		WithExec([]string{"kubectl", "rollout", "status", "deployment", "-n", "cert-manager", "cert-manager-webhook"}).
		WithExec([]string{"sh", "-c", "helm uninstall --wait --ignore-not-found klio -n cnpg-system || true"}).
		WithExec([]string{"sh", "-c", "helm upgrade -i --wait --create-namespace --namespace cnpg-system" +
			" --set controllerManager.manager.image.repository=registry.dev:5000/klio-operator-testing" +
			" --set controllerManager.manager.image.tag=dev" +
			" --set controllerManager.manager.env.SIDECAR_IMAGE=registry.dev:5000/klio-testing:dev" +
			" --set prometheus.enable=false" +
			" klio /operator/dist/chart"}).
		Terminal()

	return kubectlCtr, nil
}
