// A Helmify module

package main

import (
	"fmt"

	"dagger/helmify/internal/dagger"
)

type Helmify struct {
}

func (m *Helmify) Run(
	// source is the directory containing the source code for the project
	source *dagger.Directory,
	// Version of the kustomize image to use
	// renovate image: datasource=docker depName=registry.k8s.io/kustomize/kustomize versioning=docker
	// +default="registry.k8s.io/kustomize/kustomize:v5.8.1@sha256:899fcd3bc898160e62bcaf82932b0cb29ba38d16272353db2e7acbba82129429"
	// +optional
	kustomizeImage string,
	// Version of Helmify to use.
	// renovate: datasource=github-tags depName=arttor/helmify versioning=semver
	// +default="v0.4.20"
	// +optional
	helmifyVersion string,
) *dagger.Directory {
	ctr := dag.Container().From(kustomizeImage).
		WithMountedDirectory("/src", source).
		WithWorkdir("/src").
		WithExec([]string{
			"sh", "-c",
			fmt.Sprintf(
				"wget https://github.com/arttor/helmify/releases/download/%v/helmify_Linux_$(uname -m).tar.gz -O - | tar xz -C /app",
				helmifyVersion,
			),
		}).
		WithExec([]string{
			"sh", "-c",
			"kustomize build default | helmify -crd-dir -add-webhook-option",
		})
	return ctr.Directory("/src/chart")
}
