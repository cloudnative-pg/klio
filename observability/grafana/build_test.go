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

package main

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// testTarget and testPanel are a deliberately partial decoding of the
// generated dashboard JSON: only the fields these tests inspect.
type testTarget struct {
	Expr string `json:"expr"`
}

type testPanel struct {
	Type    string       `json:"type"`
	Title   string       `json:"title"`
	Targets []testTarget `json:"targets"`
}

type testDashboard struct {
	UID        string      `json:"uid"`
	Panels     []testPanel `json:"panels"`
	Templating struct {
		List []struct {
			Name string `json:"name"`
		} `json:"list"`
	} `json:"templating"`
}

// buildTestDashboards runs the real dashboard builders and decodes their
// JSON back into plain structs, so these tests exercise exactly what
// `go run . -output-dir dashboards` produces.
func buildTestDashboards(t *testing.T) []testDashboard {
	t.Helper()

	builders := dashboards()
	out := make([]testDashboard, 0, len(builders))
	for _, builder := range builders {
		dash, err := builder.Build()
		if err != nil {
			t.Fatalf("building dashboard: %v", err)
		}

		encoded, err := json.Marshal(dash)
		if err != nil {
			t.Fatalf("marshaling dashboard: %v", err)
		}

		var decoded testDashboard
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("unmarshaling dashboard: %v", err)
		}
		out = append(out, decoded)
	}

	return out
}

var histogramQuantilePattern = regexp.MustCompile(`histogram_quantile\([^,]+,\s*sum by \(([^)]*)\)`)

// TestHistogramQuantilePreservesLe catches a real class of bug that gcx's
// target-valid-promql lint rule cannot: it only checks PromQL syntax, so
// `histogram_quantile(0.95, sum by (path, stage) (rate(..._bucket[...])))`
// parses fine even though dropping the `le` label from the `sum by` clause
// silently breaks the quantile calculation (histogram_quantile can't find
// the bucket boundary and the panel renders no data).
func TestHistogramQuantilePreservesLe(t *testing.T) {
	for _, dash := range buildTestDashboards(t) {
		for _, p := range dash.Panels {
			for _, target := range p.Targets {
				for _, match := range histogramQuantilePattern.FindAllStringSubmatch(target.Expr, -1) {
					hasLe := false
					for _, label := range strings.Split(match[1], ",") {
						if strings.TrimSpace(label) == "le" {
							hasLe = true

							break
						}
					}
					if !hasLe {
						t.Errorf("%s: panel %q: histogram_quantile query drops the `le` label from its "+
							"`sum by (...)` clause, which breaks the quantile calculation: %s",
							dash.UID, p.Title, target.Expr)
					}
				}
			}
		}
	}
}

var rateCallPattern = regexp.MustCompile(`rate\(([^()]+)\)`)

// TestRateUsesRateInterval catches a hardcoded rate() window (e.g.
// rate(x[5m])) in place of Grafana's own $__rate_interval, which adapts to
// the dashboard's selected time range and zoom level. Every rate() call in
// this dashboard is meant to use $__rate_interval; gcx's PromQL linter
// accepts a hardcoded window just as happily, so nothing else catches this.
// increase() calls are intentionally not checked here: several panels use a
// fixed window on purpose (24h/7d trailing counts, $__range for the whole
// visible range), so there's no single correct value to assert.
func TestRateUsesRateInterval(t *testing.T) {
	for _, dash := range buildTestDashboards(t) {
		for _, p := range dash.Panels {
			for _, target := range p.Targets {
				for _, match := range rateCallPattern.FindAllStringSubmatch(target.Expr, -1) {
					if !strings.Contains(match[1], "[$__rate_interval]") {
						t.Errorf("%s: panel %q: rate() call does not use [$__rate_interval]: %s", dash.UID, p.Title, target.Expr)
					}
				}
			}
		}
	}
}

var variableRefPattern = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)

// TestQueryVariablesAreDeclared catches a typo'd template-variable reference
// (e.g. $namespac instead of $namespace), which Prometheus would otherwise
// silently treat as a literal string in the query rather than substituting
// the dashboard variable.
func TestQueryVariablesAreDeclared(t *testing.T) {
	for _, dash := range buildTestDashboards(t) {
		declared := map[string]bool{}
		for _, v := range dash.Templating.List {
			declared[v.Name] = true
		}

		for _, p := range dash.Panels {
			for _, target := range p.Targets {
				for _, match := range variableRefPattern.FindAllStringSubmatch(target.Expr, -1) {
					name := match[1]
					// Grafana's own built-in global variables (__rate_interval,
					// __range, __interval, __from, __to, ...) aren't dashboard
					// template variables and don't need to be declared.
					if strings.HasPrefix(name, "__") || name == "timeFilter" {
						continue
					}
					if !declared[name] {
						t.Errorf("%s: panel %q: query references $%s, which is not a declared dashboard "+
							"variable: %s", dash.UID, p.Title, name, target.Expr)
					}
				}
			}
		}
	}
}

// TestNoDuplicatePanelTitlesPerRow catches a copy-paste mistake: two panels
// with the same title in the same row section, which is confusing in the UI
// and usually means a panel was duplicated without renaming it.
func TestNoDuplicatePanelTitlesPerRow(t *testing.T) {
	for _, dash := range buildTestDashboards(t) {
		currentRow := ""
		seen := map[string]bool{}

		for _, p := range dash.Panels {
			if p.Type == "row" {
				currentRow = p.Title
				seen = map[string]bool{}

				continue
			}
			if seen[p.Title] {
				t.Errorf("%s: row %q: duplicate panel title %q", dash.UID, currentRow, p.Title)
			}
			seen[p.Title] = true
		}
	}
}
