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

// Stage identifies a single step in the per-block WAL pipeline, used as the
// value of the `stage` attribute to split a block-duration histogram across
// its constituent steps instead of emitting one instrument per step.
type Stage string

const (
	// StageWrap marks the step that wraps a WAL block (compression/encryption).
	StageWrap Stage = "wrap"
	// StageWrite marks the server step that writes a wrapped block to disk.
	StageWrite Stage = "write"
	// StageFlush marks the server step that flushes buffered blocks to disk.
	StageFlush Stage = "flush"
	// StageSend marks a step that sends a wrapped block over gRPC: the client
	// sending to the server (put), or the server sending to the client (get).
	StageSend Stage = "send"
	// StageRead marks the server step that reads a wrapped block from disk while
	// serving a WAL file (get).
	StageRead Stage = "read"
	// StageUnwrap marks the server step that unwraps a block (decompression/
	// decryption) while serving a WAL file (get).
	StageUnwrap Stage = "unwrap"
)

// Attribute returns the `stage` attribute for this stage.
func (s Stage) Attribute() attribute.KeyValue {
	return AttributeKeyStage.Of(string(s))
}

// Path identifies the WAL data-flow a per-block stage belongs to, used as the
// value of the `path` attribute. It distinguishes the ingest path (`put`:
// client streaming WAL into Klio) from the serve path (`get`: Klio serving WAL
// back to a recovering client), so stages that share a name across paths — such
// as `send` — remain separable.
type Path string

const (
	// PathPut marks the WAL ingest path (gRPC Put: receive, wrap, write, flush,
	// and the client-side send).
	PathPut Path = "put"
	// PathGet marks the WAL serve path (gRPC Get: read, unwrap, send).
	PathGet Path = "get"
)

// Attribute returns the `path` attribute for this path.
func (p Path) Attribute() attribute.KeyValue {
	return AttributeKeyPath.Of(string(p))
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
	// AttributeKeyStage is the attribute key for the pipeline stage of a
	// per-block WAL duration histogram (receive, wrap, write, flush, read,
	// unwrap, send).
	AttributeKeyStage AttributeKey = "stage"
	// AttributeKeyPath is the attribute key for the WAL data-flow path (put or
	// get) of a per-block WAL duration histogram.
	AttributeKeyPath AttributeKey = "path"
)

// Of builds an OTEL string attribute with the attribute key and the given value.
func (k AttributeKey) Of(value string) attribute.KeyValue {
	return attribute.String(string(k), value)
}
