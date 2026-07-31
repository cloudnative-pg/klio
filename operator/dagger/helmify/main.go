/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

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
