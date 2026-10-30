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

// The test types are a deliberately partial decoding of the generated
// dashboard resource: only the fields these tests inspect.

type testVariable struct {
	Spec struct {
		Name  string `json:"name"`
		Query struct {
			Spec struct {
				Query string `json:"query"`
			} `json:"spec"`
		} `json:"query"`
	} `json:"spec"`
}

type testElement struct {
	Spec struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		VizConfig   struct {
			Group string `json:"group"`
		} `json:"vizConfig"`
		Data struct {
			Spec struct {
				Queries []struct {
					Spec struct {
						Query struct {
							Spec struct {
								Expr string `json:"expr"`
							} `json:"spec"`
						} `json:"query"`
					} `json:"spec"`
				} `json:"queries"`
			} `json:"spec"`
		} `json:"data"`
	} `json:"spec"`
}

type testRow struct {
	Spec struct {
		Title  string `json:"title"`
		Layout struct {
			Spec struct {
				Items []struct {
					Spec struct {
						Element struct {
							Name string `json:"name"`
						} `json:"element"`
					} `json:"spec"`
				} `json:"items"`
			} `json:"spec"`
		} `json:"layout"`
	} `json:"spec"`
}

type testGroup struct {
	Spec struct {
		Title     string         `json:"title"`
		Variables []testVariable `json:"variables"`
		Layout    struct {
			Spec struct {
				Rows []testRow `json:"rows"`
			} `json:"spec"`
		} `json:"layout"`
	} `json:"spec"`
}

type testDashboard struct {
	Spec struct {
		Variables []testVariable         `json:"variables"`
		Elements  map[string]testElement `json:"elements"`
		Layout    struct {
			Spec struct {
				Rows []testGroup `json:"rows"`
			} `json:"spec"`
		} `json:"layout"`
	} `json:"spec"`
}

// testPanel is a panel with the group and row it belongs to.
type testPanel struct {
	group, row, title, description, viz string
	exprs                               []string
}

// buildTestDashboard runs the real dashboard builder and decodes its JSON
// back into plain structs, so these tests exercise exactly what
// `go run . -output dashboards/klio.json` produces.
func buildTestDashboard(t *testing.T) testDashboard {
	t.Helper()

	dash, err := build().Build()
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

	return decoded
}

// groups returns the top-level rows of the dashboard, failing the test unless
// they are the expected ones, so a layout change cannot leave the other tests
// checking nothing.
func (d testDashboard) groups(t *testing.T) []testGroup {
	t.Helper()

	titles := make([]string, 0, len(d.Spec.Layout.Spec.Rows))
	for _, g := range d.Spec.Layout.Spec.Rows {
		titles = append(titles, g.Spec.Title)
	}
	if strings.Join(titles, ",") != "Clusters,Servers" {
		t.Fatalf("top-level rows are %q, want Clusters and Servers", titles)
	}

	return d.Spec.Layout.Spec.Rows
}

// panels returns every panel of the dashboard, in layout order.
func (d testDashboard) panels(t *testing.T) []testPanel {
	t.Helper()

	var out []testPanel
	for _, g := range d.groups(t) {
		for _, row := range g.Spec.Layout.Spec.Rows {
			for _, item := range row.Spec.Layout.Spec.Items {
				el, ok := d.Spec.Elements[item.Spec.Element.Name]
				if !ok {
					t.Fatalf("group %q row %q references the missing element %q",
						g.Spec.Title, row.Spec.Title, item.Spec.Element.Name)
				}
				p := testPanel{
					group: g.Spec.Title, row: row.Spec.Title, title: el.Spec.Title,
					description: el.Spec.Description, viz: el.Spec.VizConfig.Group,
				}
				for _, q := range el.Spec.Data.Spec.Queries {
					p.exprs = append(p.exprs, q.Spec.Query.Spec.Expr)
				}
				out = append(out, p)
			}
		}
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
	for _, p := range buildTestDashboard(t).panels(t) {
		for _, expr := range p.exprs {
			for _, match := range histogramQuantilePattern.FindAllStringSubmatch(expr, -1) {
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
						p.group, p.title, expr)
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
	for _, p := range buildTestDashboard(t).panels(t) {
		for _, expr := range p.exprs {
			for _, match := range rateCallPattern.FindAllStringSubmatch(expr, -1) {
				if !strings.Contains(match[1], "[$__rate_interval]") {
					t.Errorf("%s: panel %q: rate() call does not use [$__rate_interval]: %s", p.group, p.title, expr)
				}
			}
		}
	}
}

var variableRefPattern = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)

// undeclaredVariables returns the variables expr references that are not in
// declared. Grafana's own built-in global variables (__rate_interval,
// __range, __interval, __from, __to, ...) aren't template variables and
// don't need to be declared.
func undeclaredVariables(expr string, declared map[string]bool) []string {
	var out []string
	for _, match := range variableRefPattern.FindAllStringSubmatch(expr, -1) {
		name := match[1]
		if strings.HasPrefix(name, "__") || name == "timeFilter" {
			continue
		}
		if !declared[name] {
			out = append(out, name)
		}
	}

	return out
}

// TestQueryVariablesAreDeclared catches a typo'd template-variable reference
// (e.g. $namespac instead of $namespace), which Prometheus would otherwise
// silently treat as a literal string in the query rather than substituting
// the variable. A panel can only use the dashboard variables and the
// variables of its own group: the ones of the other group do not resolve
// there.
func TestQueryVariablesAreDeclared(t *testing.T) {
	dash := buildTestDashboard(t)

	declared := map[string]map[string]bool{}
	for _, g := range dash.groups(t) {
		names := map[string]bool{}
		for _, v := range dash.Spec.Variables {
			names[v.Spec.Name] = true
		}
		for _, v := range g.Spec.Variables {
			names[v.Spec.Name] = true
		}
		declared[g.Spec.Title] = names
	}

	for _, p := range dash.panels(t) {
		for _, expr := range p.exprs {
			for _, name := range undeclaredVariables(expr, declared[p.group]) {
				t.Errorf("%s: panel %q: query references $%s, which is not a variable of the dashboard or "+
					"of its group: %s", p.group, p.title, name, expr)
			}
		}
	}
}

// TestVariableQueriesAreChained catches a variable whose query references a
// variable that is not declared before it in the same group (or at dashboard
// level), which Grafana cannot resolve when it refreshes the variable.
func TestVariableQueriesAreChained(t *testing.T) {
	dash := buildTestDashboard(t)

	for _, g := range dash.groups(t) {
		declared := map[string]bool{}
		for _, v := range dash.Spec.Variables {
			declared[v.Spec.Name] = true
		}
		for _, v := range g.Spec.Variables {
			for _, name := range undeclaredVariables(v.Spec.Query.Spec.Query, declared) {
				t.Errorf("%s: variable %q references $%s, which is not declared before it: %s",
					g.Spec.Title, v.Spec.Name, name, v.Spec.Query.Spec.Query)
			}
			declared[v.Spec.Name] = true
		}
	}
}

// TestNoDuplicatePanelTitles catches the same panel shown twice on the
// dashboard, in the same row or in another section, and a copy-paste mistake
// such as a panel duplicated without renaming it.
func TestNoDuplicatePanelTitles(t *testing.T) {
	seen := map[string]testPanel{}
	for _, p := range buildTestDashboard(t).panels(t) {
		if p.title == "" { // the description text panels
			continue
		}
		if first, ok := seen[p.title]; ok {
			t.Errorf("panel %q is both in %s/%s and in %s/%s", p.title, first.group, first.row, p.group, p.row)
		}
		seen[p.title] = p
	}
}

// TestPanelsHaveTitleAndDescription stands in for gcx's panel-title-description
// rule, which the lint task disables because the text panels that explain
// each group and row have no title: every other panel needs a title, and a
// description for its info tooltip.
func TestPanelsHaveTitleAndDescription(t *testing.T) {
	for _, p := range buildTestDashboard(t).panels(t) {
		if p.viz == "text" {
			continue
		}
		if p.title == "" {
			t.Errorf("%s: row %q: a %s panel has no title", p.group, p.row, p.viz)
		}
		if p.description == "" {
			t.Errorf("%s: row %q: panel %q has no description", p.group, p.row, p.title)
		}
	}
}

var metricPattern = regexp.MustCompile(`\b(klio_[a-z0-9_]+|cnpg_[a-z0-9_]+)`)

// TestPanelsUseGroupFilters catches a group variable that a panel query leaves
// out, so that selecting it silently has no effect on that panel. Series
// that lack the label a variable selects on apply it through a join (see
// scope).
func TestPanelsUseGroupFilters(t *testing.T) {
	dash := buildTestDashboard(t)

	variables := map[string][]string{}
	for _, g := range dash.groups(t) {
		for _, v := range g.Spec.Variables {
			variables[g.Spec.Title] = append(variables[g.Spec.Title], v.Spec.Name)
		}
	}

	for _, p := range dash.panels(t) {
		for _, expr := range p.exprs {
			metric := metricPattern.FindString(expr)
			if metric == "" { // text panels and SQL expressions
				continue
			}
			for _, v := range variables[p.group] {
				if !regexp.MustCompile(`\$` + v + `\b`).MatchString(expr) {
					t.Errorf("%s: panel %q: query does not filter by $%s: %s", p.group, p.title, v, expr)
				}
			}
		}
	}
}
