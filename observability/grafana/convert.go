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
	"fmt"

	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
)

// The panels are built with the schema v1 panel builders, whose typed
// options cover every visualization used here, then converted to schema v2
// elements the way Grafana converts a v1 dashboard: each target becomes a
// DataQuery of its data source type, and the options and field config move
// unchanged under the visualization config.

// convertPanel converts a v1 panel to a v2 panel element with the given ID.
func convertPanel(p dashboard.Panel, id int) (dashboardv2.PanelKind, error) {
	queries := make([]dashboardv2.PanelQueryKind, 0, len(p.Targets))
	for i, target := range p.Targets {
		q, err := convertTarget(target, i)
		if err != nil {
			return dashboardv2.PanelKind{}, fmt.Errorf("panel %d target %d: %w", id, i, err)
		}
		queries = append(queries, q)
	}

	fieldConfig := dashboardv2.FieldConfigSource{Overrides: []dashboardv2.Dashboardv2FieldConfigSourceOverrides{}}
	if p.FieldConfig != nil {
		if err := remarshal(p.FieldConfig, &fieldConfig); err != nil {
			return dashboardv2.PanelKind{}, fmt.Errorf("panel %d field config: %w", id, err)
		}
	}
	options := p.Options
	if options == nil {
		options = map[string]any{}
	}

	title := ""
	if p.Title != nil {
		title = *p.Title
	}

	return dashboardv2.PanelKind{
		Kind: "Panel",
		Spec: dashboardv2.PanelSpec{
			Id:          float64(id),
			Title:       title,
			Description: p.Description,
			Transparent: p.Transparent,
			Links:       []dashboardv2.DataLink{},
			Data: dashboardv2.QueryGroupKind{
				Kind: "QueryGroup",
				Spec: dashboardv2.QueryGroupSpec{
					Queries:         queries,
					Transformations: []dashboardv2.TransformationKind{},
				},
			},
			VizConfig: dashboardv2.VizConfigKind{
				Kind:  "VizConfig",
				Group: p.Type,
				Spec: dashboardv2.VizConfigSpec{
					Options:     options,
					FieldConfig: fieldConfig,
				},
			},
		},
	}, nil
}

// convertTarget converts a v1 panel target to a v2 panel query: the data
// source moves to the query envelope and the rest becomes its spec.
func convertTarget(target any, index int) (dashboardv2.PanelQueryKind, error) {
	spec := map[string]any{}
	if err := remarshal(target, &spec); err != nil {
		return dashboardv2.PanelQueryKind{}, err
	}

	group, dsName := "", ""
	if ds, ok := spec["datasource"].(map[string]any); ok {
		group, _ = ds["type"].(string)
		dsName, _ = ds["uid"].(string)
	}
	refID, _ := spec["refId"].(string)
	if refID == "" {
		refID = fmt.Sprintf("%c", 'A'+index)
	}
	hidden, _ := spec["hide"].(bool)
	delete(spec, "datasource")
	delete(spec, "refId")
	delete(spec, "hide")

	return dashboardv2.PanelQueryKind{
		Kind: "PanelQuery",
		Spec: dashboardv2.PanelQuerySpec{
			RefId:  refID,
			Hidden: hidden,
			Query: dashboardv2.DataQueryKind{
				Kind:       "DataQuery",
				Group:      group,
				Version:    "v0",
				Datasource: &dashboardv2.Dashboardv2DataQueryKindDatasource{Name: &dsName},
				Spec:       spec,
			},
		},
	}, nil
}

// remarshal copies in into out through JSON, between types that share the
// same JSON shape.
func remarshal(in, out any) error {
	encoded, err := json.Marshal(in)
	if err != nil {
		return err
	}

	return json.Unmarshal(encoded, out)
}
