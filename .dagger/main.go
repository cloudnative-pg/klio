package main

import (
	"context"
	"dagger/klio/internal/dagger"
	"time"

	"github.com/sourcegraph/conc/pool"
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

	imageTarball := m.Image(source).AsTarball()

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
		WithExec([]string{"kubectl", "apply", "--server-side", "-f", "https://raw.githubusercontent.com/cloudnative-pg/artifacts/refs/heads/main/manifests/operator-manifest.yaml"}).
		WithExec([]string{"kubectl", "rollout", "status", "deployment", "cnpg-controller-manager", "-n", "cnpg-system"}).
		// Install cert-manager and wait for it to be ready
		WithExec([]string{"kubectl", "apply", "-f", "https://github.com/cert-manager/cert-manager/releases/download/v1.16.2/cert-manager.yaml"}).
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
		WithCgoDisabled().
		Build(source, dagger.GoBuildOpts{
			Tags: []string{"viper_bind_struct"},
		})
}

// Image compiles a Klio image
func (m *Klio) Image(
	source *dagger.Directory,
) *dagger.Container {
	// Download latest Kopia binary
	kopiaBinary := dag.Container().
		From("alpine").
		WithExec([]string{"apk", "add", "curl"}).
		WithExec([]string{"curl", "-LO", "https://github.com/kopia/kopia/releases/download/v0.18.2/kopia-0.18.2-linux-x64.tar.gz"}).
		WithExec([]string{"tar", "xvzf", "kopia-0.18.2-linux-x64.tar.gz"}).
		File("kopia-0.18.2-linux-x64/kopia")

	return dag.Container().
		From("alpine").
		WithFile("/usr/bin/klio", m.Build(source)).
		WithFile("/usr/bin/kopia", kopiaBinary).
		WithEntrypoint([]string{"/app"})
}

// Lint runs golangci-lint on the source directory
func (m *Klio) Lint(
	source *dagger.Directory,
) *dagger.Container {
	return dag.GolangciLint().
		WithBuildCache(dag.CacheVolume("golangci-lint-build-cache")).
		WithLinterCache(dag.CacheVolume("golangci-lint-linter-cache")).
		WithModuleCache(dag.CacheVolume("golangci-lint-module-cache")).
		Run(source)
}

// Test runs the unit tests
func (m *Klio) Test(
	source *dagger.Directory,
) *dagger.Container {
	return dag.Go(dagger.GoOpts{Version: goVersion}).
		WithCgoDisabled().
		WithSource(source).
		Exec([]string{"go", "install", "github.com/kopia/kopia@latest"}).
		WithExec([]string{
			"go",
			"test",
			"./...",
		})
}

// CI exec the regular CI checks
func (m *Klio) CI(
	ctx context.Context,
	source *dagger.Directory,
) error {
	p := pool.New().WithContext(ctx)
	p.Go(func(ctx context.Context) error {
		_, err := m.Lint(source).Sync(ctx)
		return err
	})
	p.Go(func(ctx context.Context) error {
		_, err := m.Test(source).Sync(ctx)
		return err
	})
	return p.Wait()
}

// Protoc runs "protoc" and compiles the proto file into the relative
// client and server
func (m *Klio) Protoc(
	ctx context.Context,
	source *dagger.Directory,
) *dagger.Directory {
	return dag.ProtocGenGoGrpc().
		Run(
			source,
			"proto",
			"module=github.com/EnterpriseDB/klio/internal/klioserver/grpc",
			"module=github.com/EnterpriseDB/klio/internal/klioserver/grpc",
		)
}
