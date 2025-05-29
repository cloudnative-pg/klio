package main

import (
	"context"
	"time"

	"dagger/klio/internal/dagger"
)

const postgresPassword = "mysecretpassword"
const goVersion = "1.23.4"

type Klio struct{}

// Kubernetes starts a Kubernetes server with CNPG and Klio
func (m *Klio) Kubernetes(ctx context.Context, source *dagger.Directory) (*dagger.Container, error) {
	registrySvc := dag.Container().
		From("registry:2.8").
		WithExposedPort(5000).
		AsService()

	imageTarball := m.Image(source, true).AsTarball()

	_, err := dag.Container().From("quay.io/skopeo/stable").
		WithServiceBinding("registry", registrySvc).
		WithEnvVariable("BUST", time.Now().String()).
		WithWorkdir("/tmp").
		WithFile("/tmp/archive-file.tar", imageTarball).
		WithExec(
			[]string{
				"copy",
				"--dest-tls-verify=false",
				"docker-archive:archive-file.tar",
				"docker://registry:5000/klio:dev",
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

	return dag.Container().
		From("alpine/helm").
		WithExec([]string{"apk", "add", "kubectl", "tmux", "vim"}).
		WithEnvVariable("KUBECONFIG", "/.kube/config").
		WithFile("/.kube/config", k3s.Config()).
		WithDirectory("/kubernetes", source.Directory("kubernetes")).
		// Install CNPG and wait for it to be ready
		WithExec([]string{"kubectl", "apply", "--server-side", "-f",
			"https://raw.githubusercontent.com/cloudnative-pg/artifacts/refs/heads/main/manifests/operator-manifest.yaml"}).
		WithExec([]string{"kubectl", "rollout", "status", "deployment", "cnpg-controller-manager", "-n",
			"cnpg-system"}).
		// Install cert-manager and wait for it to be ready
		WithExec([]string{"kubectl", "apply", "-f",
			"https://github.com/cert-manager/cert-manager/releases/download/v1.16.2/cert-manager.yaml"}).
		WithExec([]string{"kubectl", "rollout", "status", "deployment", "-n", "cert-manager", "cert-manager-webhook"}).
		WithExec([]string{"kubectl", "apply", "-k", "/kubernetes/klio/overlays/dev"}).
		WithExec([]string{"kubectl", "rollout", "status", "deployment", "klio-server"}).
		WithExec([]string{"kubectl", "apply", "-f", "/kubernetes/klio-client.yaml"}), nil
}

// Build compiles and build the binary
func (m *Klio) Build(
	source *dagger.Directory,
) *dagger.File {
	return dag.Go(dagger.GoOpts{Version: goVersion}).
		WithEnvVariable("GOEXPERIMENT", "boringcrypto").
		Build(source, dagger.GoBuildOpts{
			Tags: []string{"viper_bind_struct"},
		})
}

// Image compiles a Klio image
func (m *Klio) Image(
	source *dagger.Directory,
	addKopiaBinary bool,
) *dagger.Container {
	result := dag.Container().
		From("registry.access.redhat.com/ubi9/ubi-minimal:9.5-1738816775").
		WithFile("/usr/bin/klio", m.Build(source)).
		WithEntrypoint([]string{"/app"})

	if addKopiaBinary {
		// Download latest Kopia binary
		kopiaBinary := dag.Container().
			From("alpine").
			WithExec([]string{"apk", "add", "curl"}).
			WithExec([]string{"curl", "-LO",
				"https://github.com/kopia/kopia/releases/download/v0.18.2/kopia-0.18.2-linux-x64.tar.gz"}).
			WithExec([]string{"tar", "xvzf", "kopia-0.18.2-linux-x64.tar.gz"}).
			File("kopia-0.18.2-linux-x64/kopia")

		result = result.WithFile("/usr/bin/kopia", kopiaBinary)
	}

	return result

}
