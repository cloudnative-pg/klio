package opentelemetry

import (
	"go.opentelemetry.io/otel/attribute"
)

// Tier identifies a Klio storage tier used as the value of the `tier`
// attribute on tiered metrics and spans.
type Tier string

const (
	// Tier1 identifies the tier-1 storage (local disk on the Klio server,
	// populated by the WAL gRPC ingest).
	Tier1 Tier = "tier1"
	// Tier2 identifies the tier-2 storage (remote object store, populated
	// by the consumer after WAL files are archived).
	Tier2 Tier = "tier2"
)

// Attribute returns the `tier` attribute for this tier.
func (t Tier) Attribute() attribute.KeyValue {
	return AttributeKeyTier.Of(string(t))
}

// Outcome identifies the result of an operation, used as the value of the
// `outcome` attribute to split a single counter across success and failure
// flavors instead of emitting two separate instruments.
type Outcome string

const (
	// OutcomeSuccess marks the success flavor of an operation counter.
	OutcomeSuccess Outcome = "success"
	// OutcomeFailure marks the failure flavor of an operation counter.
	OutcomeFailure Outcome = "failure"
)

// Attribute returns the `outcome` attribute for this outcome.
func (o Outcome) Attribute() attribute.KeyValue {
	return AttributeKeyOutcome.Of(string(o))
}

// AttributeKey is the key of an OTEL attribute recorded by Klio on metrics
// and spans.
type AttributeKey string

const (
	// AttributeKeyClusterName is the attribute key for the
	// PostgreSQL cluster name.
	AttributeKeyClusterName AttributeKey = "cluster_name"
	// AttributeKeyWalName is the attribute key for the WAL
	// segment file name.
	AttributeKeyWalName AttributeKey = "wal_name"
	// AttributeKeySnapshotSource is the attribute key for the Kopia
	// source descriptor of a base snapshot.
	AttributeKeySnapshotSource AttributeKey = "snapshot_source"
	// AttributeKeyOutcome is the attribute key for the outcome (success or failure)
	// of an operation.
	AttributeKeyOutcome AttributeKey = "outcome"
	// AttributeKeyFailureCategory is the attribute key for the
	// failure category of a backup that ended with outcome=failure.
	AttributeKeyFailureCategory AttributeKey = "failure_category"
	// AttributeKeyStream is the attribute key identifying a JetStream stream.
	AttributeKeyStream AttributeKey = "stream"
	// AttributeKeyTier is the attribute key for the storage tier (tier1 or tier2)
	// in a tiered metric.
	AttributeKeyTier AttributeKey = "tier"
)

// Of builds an OTEL string attribute with the attribute key and the given value.
func (k AttributeKey) Of(value string) attribute.KeyValue {
	return attribute.String(string(k), value)
}
