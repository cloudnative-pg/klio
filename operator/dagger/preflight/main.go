// A module for running Red Hat Preflight certification checks.
//
// Preflight (https://github.com/redhat-openshift-ecosystem/openshift-preflight)
// validates container images against Red Hat's certification policies. Wrapping
// it as a Dagger module lets the same checks run identically in local
// development, commit/PR CI, and the Tekton certification pipeline.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"dagger/preflight/internal/dagger"
)

// Preflight runs Red Hat Preflight certification checks.
type Preflight struct {
	// Container is the Preflight base container used to run checks.
	// +private
	Container *dagger.Container
}

// New creates a Preflight module backed by a container built from the specified
// image, or the default upstream Preflight image when none is given.
func New(
	// preflightImage overrides the Preflight container image.
	// renovate image: datasource=docker depName=preflight lookupName=quay.io/opdev/preflight versioning=docker
	// +default="quay.io/opdev/preflight:1.19.2@sha256:58dba81f9643ecfd6329075d3dbafc612db6489e31780eaad10bd249d8da0435"
	// +optional
	preflightImage string,
) *Preflight {
	return &Preflight{
		Container: dag.Container().
			From(preflightImage).
			WithEnvVariable("PFLT_ARTIFACTS", artifactsPath),
	}
}

const (
	// artifactsPath is where Preflight writes its results inside the container.
	artifactsPath = "/artifacts"

	// dockerConfigPath is where an optional registry auth file is mounted.
	dockerConfigPath = "/preflight/config.json"
)

// ErrNoResults is returned when a Preflight run produced no results.json.
var ErrNoResults = errors.New("preflight produced no results")

// ErrFailedChecks is returned when a Preflight run reported failing checks.
var ErrFailedChecks = errors.New("preflight reported failing checks")

// result is the subset of a Preflight results.json that we evaluate.
type result struct {
	Passed  bool `json:"passed"`
	Results struct {
		Failed []check `json:"failed"`
		Errors []check `json:"errors"`
	} `json:"results"`
}

// check is a single Preflight check entry.
type check struct {
	Name string `json:"name"`
}

// Artifacts is the output of a Preflight run.
type Artifacts struct {
	// Directory holds the raw Preflight artifacts (a results.json per
	// architecture).
	Directory *dagger.Directory
}

// CheckContainer runs `preflight check container` against a container image and
// returns its artifacts. Call Verdict on the result to evaluate pass/fail.
func (m *Preflight) CheckContainer(
	// image is the fully-qualified container image reference to inspect.
	image string,
	// dockerConfig is an optional docker config.json used to authenticate to
	// the registry hosting the image under test.
	// +optional
	dockerConfig *dagger.Secret,
) *Artifacts {
	ctr := m.Container
	if dockerConfig != nil {
		ctr = ctr.
			WithMountedSecret(dockerConfigPath, dockerConfig).
			WithEnvVariable("PFLT_DOCKERCONFIG", dockerConfigPath)
	}

	// Preflight exits non-zero only when it fails to run, not when checks fail,
	// so a genuine failure fails the exec and Dagger surfaces its output. The
	// pass/fail verdict for a successful run is derived from results.json.
	ctr = ctr.WithExec([]string{"check", "container", image}, dagger.ContainerWithExecOpts{
		UseEntrypoint: true,
	})

	return &Artifacts{
		Directory: ctr.Directory(artifactsPath),
	}
}

// Verdict reads every results.json in the artifacts directory in-engine and
// returns a per-architecture PASS/FAIL summary. It errors if any check failed,
// or if Preflight produced no results (for example when the image cannot be
// pulled). Preflight exits zero even when checks fail, so this turns a failing
// result into a non-zero exit; the summary is carried inside the error on
// failure, since Dagger drops a function's return value when it also errors.
func (a *Artifacts) Verdict(ctx context.Context) (string, error) {
	paths, err := a.Directory.Glob(ctx, "**/results.json")
	if err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", ErrNoResults
	}

	var summary strings.Builder
	passed := true
	for _, path := range paths {
		contents, err := a.Directory.File(path).Contents(ctx)
		if err != nil {
			return "", err
		}

		var r result
		if err := json.Unmarshal([]byte(contents), &r); err != nil {
			return "", fmt.Errorf("parsing %s: %w", path, err)
		}

		if r.Passed {
			fmt.Fprintf(&summary, "PASS %s\n", path)
			continue
		}

		passed = false
		fmt.Fprintf(&summary, "FAIL %s\n", path)
		for _, c := range append(r.Results.Failed, r.Results.Errors...) {
			fmt.Fprintf(&summary, "  - %s\n", c.Name)
		}
	}

	if !passed {
		return "", fmt.Errorf("%w:\n%s", ErrFailedChecks, summary.String())
	}
	return summary.String(), nil
}
