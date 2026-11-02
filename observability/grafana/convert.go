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

	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2beta1"
)

// vizConfig adapts a visualization builder to a schema v2 panel. The SDK's
// visualization builders (stat, timeseries, table, text) build a
// dashboardv2beta1.VizConfigKind, which dashboardv2.PanelBuilder does not
// take, although the two types share the same fields.
//
// TODO: delete this file once the SDK's visualization builders build a
// dashboardv2.VizConfigKind (up to v0.0.21 they only build the
// dashboardv2beta1 one), and pass them to dashboardv2.PanelBuilder directly.
type vizConfig struct {
	viz cog.Builder[dashboardv2beta1.VizConfigKind]
}

// Build returns the visualization as a schema v2 VizConfigKind.
func (v vizConfig) Build() (dashboardv2.VizConfigKind, error) {
	beta, err := v.viz.Build()
	if err != nil {
		return dashboardv2.VizConfigKind{}, err
	}

	var out dashboardv2.VizConfigKind
	encoded, err := json.Marshal(beta)
	if err != nil {
		return dashboardv2.VizConfigKind{}, err
	}

	return out, json.Unmarshal(encoded, &out)
}
