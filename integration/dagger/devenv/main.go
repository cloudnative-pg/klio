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

func (m *Devenv) Kubernetes(ctx context.Context, source *dagger.Directory) (*dagger.Container, error) {
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
		From("registry:2.8").
		WithExposedPort(5000).
		AsService()

	_, err := dag.Container().From("quay.io/skopeo/stable").
		WithServiceBinding("registry", registrySvc).
		WithEnvVariable("BUST", time.Now().String()).
		WithWorkdir("/tmp").
		WithFile("/tmp/klio_tarball.tar", klioImage.AsTarball()).
		WithFile("/tmp/klio_op_tarball.tar", operatorImage.AsTarball()).
		WithExec(
			[]string{
				"copy",
				"--dest-tls-verify=false",
				"docker-archive:klio_tarball.tar",
				"docker://registry:5000/klio:dev",
			}, dagger.ContainerWithExecOpts{UseEntrypoint: true},
		).
		WithExec(
			[]string{
				"copy",
				"--dest-tls-verify=false",
				"docker-archive:klio_op_tarball.tar",
				"docker://registry:5000/klio-op:dev",
			}, dagger.ContainerWithExecOpts{UseEntrypoint: true},
		).Sync(ctx)

	if err != nil {
		return nil, err
	}

	k3s := dag.K3S("k3s-test", dagger.K3SOpts{
		Image: "rancher/k3s:v1.31.4-k3s1",
	})
	_, err = k3s.
		With(func(k *dagger.K3S) *dagger.K3S {
			return k.WithContainer(
				k.Container().
					WithEnvVariable("BUST", time.Now().String()).
					WithExec([]string{"sh", "-c", `
cat <<EOF > /etc/rancher/k3s/registries.yaml
mirrors:
  "registry:5000":
    endpoint:
      - "http://registry:5000"
EOF`}).
					WithServiceBinding("registry", registrySvc),
			)
		}).Server().Start(ctx)
	if err != nil {
		return nil, err
	}

	kubectlCtr := dag.Container().From("alpine/k8s:1.33.2").
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
		WithNewFile("/operator/config/dev/kustomization.yaml",
			`
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: cnpg-system
resources:
- ../default

images:
- name: controller
  newTag: dev
  newName: registry:5000/klio-op
`).
		WithExec([]string{"kubectl", "apply", "-k", "/operator/config/dev"}).
		Terminal()

	return kubectlCtr, nil
}
