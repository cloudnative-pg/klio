package opentelemetry

import "go.opentelemetry.io/otel/attribute"

// TierAttribute returns an attribute for the storage tier (tier1/tier2).
func TierAttribute(tier string) attribute.KeyValue {
	return attribute.String("tier", tier)
}
