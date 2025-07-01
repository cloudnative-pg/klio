package main

import (
	"fmt"

	"dagger/kubebuilder/internal/dagger"
)

type Kubebuilder struct {
	// +private
	Ctr *dagger.Container
}

func New(
	// Golang image to use.
	// renovate image: datasource=docker depName=golang versioning=docker
	// +default="golang:1.24.4-alpine"
	// +optional
	Image string,
	// renovate: datasource=git-refs depName=kubebuilder lookupName=https://github.com/kubernetes-sigs/kubebuilder versioning=semver
	// +default="v4.6.0"
	// +optional
	kubebuilderVersion string,
) *Kubebuilder {
	kubebuilderUrl := fmt.Sprintf("https://github.com/kubernetes-sigs/kubebuilder/releases/download/%v/kubebuilder_$(go env GOOS)_$(go env GOARCH)",
		kubebuilderVersion)
	return &Kubebuilder{
		Ctr: dag.Container().From(Image).WithExec([]string{"sh", "-c", "wget -O kubebuilder " + kubebuilderUrl}).
			WithExec([]string{"chmod", "+x", "kubebuilder"}).
			WithExec([]string{"mv", "kubebuilder", "/usr/local/bin/"}),
	}
}

func (m *Kubebuilder) Helm(
	source *dagger.Directory,
	// +optional
	// +default="false"
	// Whether to run the kubebuilder command with the --force flag.
	force bool,
) *dagger.Directory {
	kubebuilderCmd := []string{"kubebuilder", "edit", "--plugins=helm/v1-alpha"}
	if force {
		kubebuilderCmd = append(kubebuilderCmd, "--force")
	}
	ctr := m.Ctr.
		WithMountedDirectory("/src", source).
		WithWorkdir("/src").
		WithExec(kubebuilderCmd)
	return ctr.Directory("/src/dist")
}
