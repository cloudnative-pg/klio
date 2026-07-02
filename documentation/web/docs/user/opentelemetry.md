---
sidebar_position: 8
---

# OpenTelemetry Observability

Klio provides built-in support for [OpenTelemetry](https://opentelemetry.io/),
enabling comprehensive observability through distributed tracing and metrics
collection. This allows you to monitor backup operations, performance
characteristics, and system health across your Klio deployment.

## Available Telemetry

Klio automatically collects the following:

- Traces
   - Distributed WAL streaming and processing
   - Backup lifecycle (backup, backup run, verification, maintenance)
- Metrics
   - Server
      - Server uptime
      - Backup metrics
         - Number of snapshots
         - Number of files in the latest snapshot
         - Number of directories in the latest snapshot
         - Size of the latest snapshot
         - Timestamp of the latest snapshot
         - Timestamp of the oldest snapshot
         - Number of retained PostgreSQL backups (per cluster and tier)
         - Start/end time, timeline and LSN of the latest and oldest
           retained PostgreSQL backup (per cluster and tier)
         - Total number of backup verifications (split by outcome and tier)
      - WAL processing metrics
         - Number of WAL files written
         - Bytes written
         - Timestamp of the most recently written WAL file
         - LSN progress of WAL ingestion (Tier 1) and archival (Tier 2)
         - Timeline of the latest WAL on Tier 1 and Tier 2
         - Per-block processing durations split by stage (histogram)
         - Per-file get and Tier 2 upload durations (histogram)
      - Queue metrics
         - Number of messages in the queue
         - Number of bytes in the queue
      - [GRPC metrics](https://opentelemetry.io/docs/specs/semconv/rpc/rpc-metrics/)
      - [Go runtime statistics](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/runtime)
      - [Host metrics](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/host)
      - [Controller runtime metrics](https://book.kubebuilder.io/reference/metrics-reference)
   - Sidecar
      - Backup metrics
         - Number of backups currently in progress
         - Timestamp of the most recent backup start
         - Timestamp of the most recent successful completion
         - Timestamp of the most recent failure
         - Duration of the most recent backup
         - Total number of backup runs (split by outcome)
         - Total number of backup verifications (split by outcome)
      - [GRPC metrics](https://opentelemetry.io/docs/specs/semconv/rpc/rpc-metrics/)
      - [Go runtime statistics](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/runtime)
      - [Host metrics](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/host)
      - [Controller runtime metrics](https://book.kubebuilder.io/reference/metrics-reference)
   - Client (WAL streaming)
      - Per-block WAL send durations (histogram)
      - Timeline currently being streamed (gauge)

:::note
Log exporters are not currently supported.
:::

## Traces Reference

### Backup lifecycle spans

When a backup is triggered through CNPG-I, Klio creates
the following spans under the `klio.plugin.backup` tracer:

| Span Name | Description |
|---|---|
| `backup` | Root span covering the entire backup operation (run + verify) |
| `backup_run` | Child span for the actual data backup execution |
| `backup_verify` | Child span for post-backup verification |

The `backup` span includes the following attributes:

| Attribute | Type | Description |
|---|---|---|
| `backup.name` | string | Name assigned to the backup |

On failure, the span records the error and sets its status to
`ERROR`.

### WAL streaming spans

Klio traces WAL streaming at the per-file level; the per-block stage
timings that used to be spans are now recorded as the
[WAL duration histograms](#wal-duration-histograms-server) instead.

| Span Name | Tracer | Description |
|---|---|---|
| `download_history_file` | `klio.client.wal` | Span for downloading a timeline history file (rare). |
| `get_wal` | `klio.server.wal` | Per-file span for the gRPC Get of a WAL file (served from tier-1 or tier-2). |
| `tier2_upload` | `klio.server.consumer` | One span per WAL file archived to tier-2 (remote storage). |

## Metrics Reference

Klio metric names follow the
`klio.<component>.<domain>.<measurement>` taxonomy. The component
segment identifies which process emits the metric:

- `plugin` — the CNPG plugin sidecar running in each PostgreSQL pod.
- `server` — the Klio server StatefulSet (hosts the Kopia server, the
  WAL gRPC ingest, the embedded NATS JetStream queue, and the tier-2
  WAL consumer).
- `operator` — the Klio operator deployment. Bridges
  controller-runtime Prometheus metrics to OTLP and adds Go
  runtime and host instrumentation.
- `client` — the WAL streaming client that ships WAL from
  PostgreSQL to the Klio server. Emits the per-block WAL send
  duration histogram and the currently streamed timeline gauge.

### Attributes

Klio metrics carry the following attributes. Each per-metric table
below repeats the applicable attributes in its descriptions; this
section is the central reference for the value space of each
attribute key.

| Attribute | Values | Applies to |
|---|---|---|
| `tier` | `tier1` (local disk on the Klio server), `tier2` (remote object store) | All `klio.server.wal.*` and `klio.server.backup.*` instruments. |
| `cluster_name` | Name of the PostgreSQL cluster the recording belongs to | All `klio.server.wal.*` instruments (counters, gauges, and the WAL duration histograms), `klio.client.wal.*`, the `klio.server.backup.*` PostgreSQL backup gauges (`backups`, `latest_backup_*`, `oldest_backup_*`) and the `klio.server.backup.relay` / `klio.server.backup.maintenance` counters. |
| `outcome` | `success`, `failure` | `klio.plugin.backup.runs`, `klio.server.backup.relay`, `klio.server.backup.maintenance`, `klio.server.backup.verifications`, and all WAL duration histograms (`klio.server.wal.*_duration`, `klio.client.wal.block_duration`). |
| `failure_category` | `repository_error`, `source_error`, `verification`, `timeout`, `canceled`, `unknown` | `klio.plugin.backup.runs` failure data points only. |
| `path` | `put` (WAL ingest), `get` (WAL serve) | `klio.server.wal.block_duration`, `klio.client.wal.block_duration`. |
| `stage` | put: `wrap`, `write`, `flush`, `send` (client); get: `read`, `unwrap`, `send` | `klio.server.wal.block_duration`, `klio.client.wal.block_duration`. |
| `snapshot_source` | Kopia source descriptor (`userName@hostName:path`) | All `klio.server.backup.*` base snapshot gauges (`snapshots`, `latest_snapshot_*`, `oldest_snapshot_timestamp`). |
| `stream` | JetStream stream name (`klio-wal-stream`, `klio-backup-stream`, `klio-latest-uploaded-wal-per-cluster-stream`) | `klio.server.queue.messages`, `klio.server.queue.bytes`. |

### Backup lifecycle metrics (plugin sidecar)

These metrics are emitted by the plugin sidecar and track backup
operations on each PostgreSQL instance:

| Metric Name | Type | Unit | Description |
|---|---|---|---|
| `klio.plugin.backup.in_progress` | UpDownCounter | `{backups}` | Number of backups currently in progress |
| `klio.plugin.backup.latest_start_time` | Gauge | s | Unix epoch timestamp when the most recent backup started |
| `klio.plugin.backup.latest_completion_time` | Gauge | s | Unix epoch timestamp when the most recent backup completed successfully |
| `klio.plugin.backup.latest_failure_time` | Gauge | s | Unix epoch timestamp when the most recent backup failed |
| `klio.plugin.backup.latest_duration` | Gauge | s | Duration of the most recent backup |
| `klio.plugin.backup.duration` | Histogram | s | Distribution of backup durations, split by the `outcome` attribute (`success` / `failure`) |
| `klio.plugin.backup.runs` | Counter | `{backups}` | Total number of backup runs, split by the `outcome` attribute (`success` / `failure`). Failure data points additionally carry a `failure_category` attribute classifying the failure. Backup verification is part of a run: a verification failure is recorded here with `failure_category="verification"`, and a clean verification is included in the `outcome="success"` count |

The `failure_category` attribute on `klio.plugin.backup.runs` failure
data points takes one of the following values:

- `repository_error` — the backup failed while interacting with the
  Klio server or the Kopia repository.
- `source_error` — the backup failed while connecting to or interacting
  with the source PostgreSQL instance.
- `verification` — tier-1 verification detected corruption in the
  freshly taken backup.
- `timeout` — the backup exceeded its deadline.
- `canceled` — the backup's context was canceled before a more specific
  category could be determined. This covers cluster restart,
  hibernation, pod eviction, and client disconnect; the metric does not
  distinguish between them.
- `unknown` — the failure did not match any of the categories above.

:::note
These metrics are tied to the plugin sidecar lifecycle: when the
sidecar restarts (for example, after a pod reschedule or PostgreSQL
instance failover) the counters reset to zero and the gauges are
re-initialized on the next backup. As a result,
`klio.plugin.backup.runs` reports totals since the last sidecar start
rather than over the life of the cluster, and may diverge from the
count of `Backup` resources.
:::

### WAL ingest metrics (server)

The WAL ingest series is unified across tiers: WAL bytes and files
written to local disk by the WAL gRPC server (tier 1) and uploaded
to remote storage by the consumer (tier 2) share a single instrument
family and are distinguished by the `tier` attribute (`"tier1"` or
`"tier2"`).

| Metric Name | Type | Unit | Description |
|---|---|---|---|
| `klio.server.wal.written_size` | Counter | By | Number of bytes written for WAL files (per tier) |
| `klio.server.wal.written` | Counter | - | Number of WAL files written (per tier) |
| `klio.server.wal.latest_written_time` | Gauge | s | Unix epoch timestamp of the most recently written WAL file (per tier) |
| `klio.server.wal.latest_written_lsn` | Gauge | By | LSN of the most recently written WAL byte. On tier 1 this is the flush pointer (matches `pg_current_wal_flush_lsn()` semantics); on tier 2 this is the last byte of the most recently archived WAL segment |
| `klio.server.wal.latest_written_timeline` | Gauge | - | Timeline ID of the most recently completed WAL file (per tier) |

Every recording carries a `cluster_name` attribute identifying the
PostgreSQL cluster, alongside the `tier` discriminator.

### Post-backup processing metrics (server)

The tier-1 backup itself is taken and counted client-side
(`klio.plugin.backup.runs`). Afterwards the server does two kinds of work for
the completed backup: optionally **relays** it to tier-2 (migration +
verification), and runs **maintenance** (base-snapshot retention + WAL
cleanup) on each tier. The relay is counted by `klio.server.backup.relay`;
maintenance is counted by `klio.server.backup.maintenance`, discriminated by a
`tier` attribute (`tier1` / `tier2`).

Both carry `cluster_name` and `outcome` and are recorded once per attempt, so
a backup whose relay or maintenance is retried produces multiple data points
before it succeeds or is dead-lettered.

| Metric Name | Type | Unit | Description |
|---|---|---|---|
| `klio.server.backup.relay` | Counter | `{relays}` | Number of tier-2 relay attempts after a backup (migration to tier-2 and verification), split by `cluster_name` and `outcome` (`success` / `failure`). |
| `klio.server.backup.maintenance` | Counter | `{runs}` | Number of maintenance runs after a backup (base-snapshot retention and WAL cleanup), split by `cluster_name`, `tier` (`tier1` / `tier2`) and `outcome` (`success` / `failure`) |

### WAL duration histograms (server)

The server records WAL processing latencies as OpenTelemetry
histograms. They replace the per-block spans Klio previously emitted
for each WAL stage, which were impractical for distributions: the
histograms can be aggregated across clusters and rendered as
percentile dashboards (`p50`, `p95`, `p99`) over time. Per-block
stages are recorded once per WAL block; the per-file instruments are
recorded once per WAL file.

| Metric Name | Type | Unit | Description |
|---|---|---|---|
| `klio.server.wal.block_duration` | Histogram | ns | Per-block processing duration, split by the `path` (`put` ingest / `get` serve), `stage`, and `outcome` attributes. Put stages: `wrap`, `write`, `flush`; get stages: `read`, `unwrap`, `send`. Per-block send latency lives on the `send` stage (client `block_duration` for ingest, server `path="get"` for serve). Carries `tier` (`tier1` for put; `tier1` or `tier2` for get, depending on which WAL server handled it) and `cluster_name` |
| `klio.server.wal.get_duration` | Histogram | ns | Per-file duration of the gRPC get of a complete WAL file, split by `outcome`. Carries `tier` (`tier1` or `tier2`, depending on which WAL server served it) and `cluster_name` |
| `klio.server.wal.upload_duration` | Histogram | ns | Per-file duration of the tier-2 archival upload to remote storage, split by `outcome`. Carries `tier="tier2"` and `cluster_name` |

The bucket boundaries are explicit (rather than an exponential
aggregation) so they survive export through the Prometheus bridge, and
are an initial set expected to be refined against real distributions.

### WAL duration histograms (client)

The WAL streaming client records the latency of shipping each WAL
block to the Klio server.

| Metric Name | Type | Unit | Description |
|---|---|---|---|
| `klio.client.wal.block_duration` | Histogram | ns | Per-block duration of the gRPC send of a WAL block to the server. Carries `path="put"`, `stage="send"`, `cluster_name`, split by `outcome` |

### WAL streaming state (client)

The WAL streaming client also exposes the timeline it is currently
streaming. It is set when replication starts and updated on each
timeline switch (failover), giving a client-side, lag-free counterpart
to the server-side `klio.server.wal.latest_written_timeline` gauge
(which only advances once new-timeline WAL is written).

| Metric Name | Type | Unit | Description |
|---|---|---|---|
| `klio.client.wal.timeline` | Gauge | - | Timeline ID the WAL streaming client is currently streaming. Carries `cluster_name` |

### Backup verification metrics (server)

As part of processing each backup the server verifies it. Verification
happens at two points: once against the tier-1 local copy, and again
against the tier-2 remote copy after migration. The `tier` attribute
(`"tier1"` or `"tier2"`) identifies which check the recording refers to.

| Metric Name | Type | Unit | Description |
|---|---|---|---|
| `klio.server.backup.verifications` | Counter | `{verifications}` | Number of backup verifications, split by the `outcome` attribute (`success` / `failure`; `failure` indicates corruption detected) and the `tier` attribute |

### Alerting on stalled WAL processing

The same `klio.server.wal.latest_written_time` instrument is emitted
from two stages of the WAL pipeline, distinguished by the `tier`
attribute. A stale value signals a different failure depending on
the tier:

- **`tier="tier1"`** reflects when the Klio server last received a WAL
  file from PostgreSQL streaming replication and persisted it to
  local disk. A stale value means PostgreSQL is no longer shipping
  WALs to Klio, which may indicate a replication problem, or that
  writing on disk is failing.

- **`tier="tier2"`** reflects when the consumer last uploaded a WAL file
  to tier-2 object storage. A stale value means the remote backend
  is no longer receiving WALs, even though PostgreSQL replication
  may still be working, or that uploading the WAL to the object store
  is failing.

### Server metrics

| Metric Name | Type | Unit | Description |
|---|---|---|---|
| `klio.server.uptime` | Gauge | s | Klio server uptime in seconds |

The `klio.server.wal.latest_written_lsn` instrument provides a
complementary view of the same two pipeline stages, expressed as a
byte offset rather than a wall-clock timestamp:

- **`tier="tier1"`** is updated on every flushed WAL block received by
  the WAL server (tracks `pg_current_wal_flush_lsn()` semantics).

- **`tier="tier2"`** is updated once per completed WAL file by the
  consumer. Its value is the LSN of the last byte of the WAL
  segment just archived.

The companion `klio.server.wal.latest_written_timeline` gauge
exposes the timeline ID of the WAL file each tier is currently
handling.

:::warning
While usually increasing, the LSN gauge may decrease after the
promotion of a lagging standby.
:::

Use these gauges alongside the timestamp gauges to distinguish a
slow pipeline (timestamps advancing, LSN gap growing) from a
stalled one (timestamps and LSN both frozen).

### Base backup metrics (server)

These metrics are emitted by the Klio server base backup component
and track Kopia snapshot statistics:

| Metric Name | Type | Unit | Description |
|---|---|---|---|
| `klio.server.backup.snapshots` | Gauge | - | Total number of base snapshots |
| `klio.server.backup.latest_snapshot_size` | Gauge | By | Size of latest base snapshot in bytes (ignoring compression and deduplication) |
| `klio.server.backup.latest_snapshot_files` | Gauge | - | Number of files in latest base snapshot |
| `klio.server.backup.latest_snapshot_dirs` | Gauge | - | Number of directories in latest base snapshot |
| `klio.server.backup.latest_snapshot_timestamp` | Gauge | s | Unix epoch timestamp of the latest base snapshot |
| `klio.server.backup.oldest_snapshot_timestamp` | Gauge | s | Unix epoch timestamp of the oldest base snapshot |

Every recording carries a `tier` attribute (`tier1` for the local
disk repository, `tier2` for the remote object store) and a
`snapshot_source` attribute identifying the source descriptor
(`userName@hostName:path`) the snapshot belongs to.

The following metrics describe the retention window of physical
PostgreSQL backups, derived from the snapshotted backup metadata.
Each recording carries a `tier` attribute and a `cluster_name`
attribute identifying the PostgreSQL cluster the backup belongs to.
The `latest_backup_*` and `oldest_backup_*` gauges describe the most
recent and oldest backup retained on that tier (a base backup cannot
span a timeline switch, so its start and end share one timeline):

| Metric Name | Type | Unit | Description |
|---|---|---|---|
| `klio.server.backup.backups` | Gauge | - | Number of PostgreSQL backups retained |
| `klio.server.backup.latest_backup_start_time` | Gauge | s | Unix epoch timestamp when the latest retained backup started |
| `klio.server.backup.latest_backup_completion_time` | Gauge | s | Unix epoch timestamp when the latest retained backup completed |
| `klio.server.backup.latest_backup_start_lsn` | Gauge | By | Start LSN of the latest retained backup (base 10) |
| `klio.server.backup.latest_backup_end_lsn` | Gauge | By | End LSN of the latest retained backup (base 10) |
| `klio.server.backup.latest_backup_timeline` | Gauge | - | Timeline of the latest retained backup |
| `klio.server.backup.oldest_backup_start_time` | Gauge | s | Unix epoch timestamp when the oldest retained backup started |
| `klio.server.backup.oldest_backup_completion_time` | Gauge | s | Unix epoch timestamp when the oldest retained backup completed |
| `klio.server.backup.oldest_backup_start_lsn` | Gauge | By | Start LSN of the oldest retained backup (base 10) |
| `klio.server.backup.oldest_backup_end_lsn` | Gauge | By | End LSN of the oldest retained backup (base 10) |
| `klio.server.backup.oldest_backup_timeline` | Gauge | - | Timeline of the oldest retained backup |

### Queue metrics (server)

These metrics are emitted by the Klio server and track the state of
the embedded NATS JetStream streams used for asynchronous Tier 2
offloading of WAL files and backups. Each sample carries a `stream`
attribute identifying the source stream — typically
`klio-wal-stream` (WAL work queue), `klio-backup-stream` (backup work
queue), and `klio-latest-uploaded-wal-per-cluster-stream` (retention
safeguard, capped to one message per cluster):

| Metric Name | Type | Unit | Description |
|---|---|---|---|
| `klio.server.queue.messages` | Gauge | - | Number of messages currently stored in the JetStream stream identified by `stream` |
| `klio.server.queue.bytes` | Gauge | By | Number of bytes currently stored in the JetStream stream identified by `stream` |

### Migration from the previous metric names

Klio is in alpha and previously emitted metrics under a flat
namespace. The component-based taxonomy above replaces those names in
a single hard rename — there is no dual emission. Update dashboards
and alerts according to the following table:

| Previous name | New name | Notes                                                                                                                                                                                                                                                                                                                                                                                                        |
|---|---|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `klio.backup.*` | `klio.plugin.backup.*` | Plugin sidecar metrics.                                                                                                                                                                                                                                                                                                                                                                                      |
| `klio.backup.running` | `klio.plugin.backup.in_progress` | Renamed and switched from a 0/1 gauge to an UpDownCounter; reports the number of concurrent backups in progress.                                                                                                                                                                                                                                                                                             |
| `klio.backup.latest_duration_seconds` | `klio.plugin.backup.latest_duration` | The `_seconds` suffix was dropped — the unit (`s`) is conveyed via the OpenTelemetry metric metadata, per [semantic conventions guidelines](https://opentelemetry.io/docs/specs/semconv/general/metrics/#units). The Prometheus export name is unchanged (`klio_plugin_backup_latest_duration_seconds`) because the Prometheus exporter appends the unit suffix when the OpenTelemetry name lacks it. |
| `klio.backup.successes`, `klio.backup.failures` | `klio.plugin.backup.runs` | Collapsed into a single counter with an `outcome` attribute (`success` / `failure`).                                                                                                                                                                                                                                                                                                                         |
| `klio.backup.verifications` | `klio.plugin.backup.runs` | Verification is part of a backup run; a clean verification is part of the `outcome="success"` count.                                                                                                                                                                                                                                                                                                          |
| `klio.backup.verification_failures` | `klio.plugin.backup.runs{outcome="failure",failure_category="verification"}` | Verification corruption is recorded as a run failure with `failure_category="verification"`.                                                                                                                                                                                                                                                                                                                 |
| `klio.wal.written_size` | `klio.server.wal.written_size` | Carries `tier="tier1"`.                                                                                                                                                                                                                                                                                                                                                                                      |
| `klio.wal.written` | `klio.server.wal.written` | Carries `tier="tier1"`.                                                                                                                                                                                                                                                                                                                                                                                      |
| `klio.wal.latest_written_time` | `klio.server.wal.latest_written_time` | Carries `tier="tier1"`.                                                                                                                                                                                                                                                                                                                                                                                      |
| `klio.wal.latest_written_lsn` | `klio.server.wal.latest_written_lsn` | Carries `tier="tier1"`.                                                                                                                                                                                                                                                                                                                                                                                      |
| `klio.wal.latest_written_timeline` | `klio.server.wal.latest_written_timeline` | Carries `tier="tier1"`.                                                                                                                                                                                                                                                                                                                                                                                      |
| `klio.consumer.written_size` | `klio.server.wal.written_size` | Folded into the unified WAL series with `tier="tier2"`.                                                                                                                                                                                                                                                                                                                                                      |
| `klio.consumer.written` | `klio.server.wal.written` | Folded into the unified WAL series with `tier="tier2"`.                                                                                                                                                                                                                                                                                                                                                      |
| `klio.consumer.latest_written_time` | `klio.server.wal.latest_written_time` | Folded into the unified WAL series with `tier="tier2"`.                                                                                                                                                                                                                                                                                                                                                      |
| `klio.consumer.latest_written_lsn` | `klio.server.wal.latest_written_lsn` | Folded into the unified WAL series with `tier="tier2"`.                                                                                                                                                                                                                                                                                                                                                      |
| `klio.consumer.latest_written_timeline` | `klio.server.wal.latest_written_timeline` | Folded into the unified WAL series with `tier="tier2"`.                                                                                                                                                                                                                                                                                                                                                      |
| `klio.consumer.backup_verification_success` | `klio.server.backup.verifications{outcome="success"}` | Moved under `server.backup` to pair with `plugin.backup`; collapsed into a single counter with an `outcome` attribute.                                                                                                                                                                                                                                                                                       |
| `klio.consumer.backup_verification_failure` | `klio.server.backup.verifications{outcome="failure"}` | Same as above.                                                                                                                                                                                                                                                                                                                                                                                               |
| `klio.base.uptime` | `klio.server.uptime` | Server-level metric, not tied to Kopia.                                                                                                                                                                                                                                                                                                                                                                      |
| `klio.base.*` | `klio.server.backup.*` | Snapshot metrics, folded under `server.backup` alongside the verification counters.                                                                                                                                                                                                                                                                                                                          |
| `klio.queue.*` | `klio.server.queue.*` | NATS JetStream metrics. Values are now reported per stream via a `stream` attribute instead of a single global aggregate.                                                                                                                                                                                                                                                                                    |

## Configuration

Klio automatically detects OpenTelemetry configuration through standard
environment variables. If no OpenTelemetry environment variables are present,
Klio will use no-op providers that don't collect any telemetry data.

Traces and metrics exporters can be configured independently through the
[`autoexport`](https://go.opentelemetry.io/contrib/exporters/autoexport) package.

### General Settings

The following environment variables are used to configure OpenTelemetry:

- `OTEL_SERVICE_NAME`: (required) Name of the service, e.g., `klio-server`
- `OTEL_RESOURCE_ATTRIBUTES`: Comma-separated list of resource attributes
  (e.g., `deployment.environment=production,service.namespace=klio-system`)
- `OTEL_RESOURCE_DETECTORS`: Comma-separated list of resource detectors
  from the [`autodetect`](https://pkg.go.dev/go.opentelemetry.io/contrib/detectors/autodetect)
  package, used to automatically populate resource attributes

### Traces exporter

To enable the traces exporter, set the `OTEL_TRACES_EXPORTER` environment
variable to one of the supported exporters:

- `otlp`: OpenTelemetry Protocol (OTLP) exporter
- `console`: Console exporter (useful for debugging)
- `none`: No-op exporter (disables tracing)

You can define the OTLP protocol using the `OTEL_EXPORTER_OTLP_TRACES_PROTOCOL`
variable, or the general `OTEL_EXPORTER_OTLP_PROTOCOL`. Supported protocols
include:

- `http/protobuf` (default)
- `grpc`

Additional configuration options for trace exporters can be found in the
documentation of the respective exporters:

- [OTLP Trace gRPC Exporter](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc)
- [OTLP Trace HTTP Exporter](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp)

### Metrics Exporter

To enable the metrics exporter, set the `OTEL_METRICS_EXPORTER` environment
variable to one of the supported exporters:

- `otlp`: OpenTelemetry Protocol (OTLP) exporter
- `prometheus`: Prometheus exporter + HTTP server
- `console`: Console exporter (useful for debugging)
- `none`: No-op exporter (disables metrics)

You can define the OTLP protocol using the `OTEL_EXPORTER_OTLP_METRICS_PROTOCOL`
variable, or the general `OTEL_EXPORTER_OTLP_PROTOCOL`. Supported protocols
include:

- `http/protobuf` (default)
- `grpc`

Additional configuration options for metrics exporters can be found in the
documentation of the respective exporters:

- [OTLP Metric gRPC Exporter](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc)
- [OTLP Metric HTTP Exporter](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp)

For the Prometheus exporter, you can configure the host and port of the HTTP
server using the following environment variables:

- `OTEL_EXPORTER_PROMETHEUS_HOST` (default: `localhost`)
- `OTEL_EXPORTER_PROMETHEUS_PORT` (default: `9464`)

### Exporters and receivers

The OTLP exporter pushes telemetry to any OTLP-compatible receiver. Common
options include:

- An [OpenTelemetry Collector](https://opentelemetry.io/docs/collector/),
  which can receive OTLP data and fan it out to multiple backends
  (Prometheus, Jaeger, Grafana, etc.). In Kubernetes, the
  [OpenTelemetry Operator](https://opentelemetry.io/docs/platforms/kubernetes/operator/)
  manages collectors via the `OpenTelemetryCollector` CRD and can expose
  a stable in-cluster OTLP endpoint for Klio to target.
- Any backend with native OTLP support.

The Prometheus exporter starts a local HTTP server that Prometheus scrapes
directly, with no intermediate collector required.

## Configuring Klio with OpenTelemetry in Kubernetes

When running in a Kubernetes environment, Klio will automatically define
`CONTAINER_NAME`, `POD_NAME` and `NAMESPACE_NAME` environment variables.
When any of these environment variables are set, Klio will automatically add
the corresponding resource attributes (`k8s.container.name`, `k8s.pod.name`,
`k8s.namespace.name`) to all telemetry data. Each attribute is added
independently - you don't need all three environment variables to be present.

:::info
If you have already defined any of these attributes in
`OTEL_RESOURCE_ATTRIBUTES`, Klio will **not override** them. Only missing
attributes will be added from the environment variables. This allows you to
customize the values while still benefiting from automatic defaults for any
attributes you don't explicitly set.
:::

### Klio server with OpenTelemetry

When deploying a Klio `Server`, you can configure OpenTelemetry by specifying
the necessary settings in the `template` section of the `Server` spec:

1. Set the required environment variables for OpenTelemetry configuration in
   the `server` container.
1. Mount any necessary TLS certificates for secure communication with the
   OpenTelemetry Collector.

For simpler management, use a `ConfigMap` to store the OpenTelemetry configuration:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: klio-otel-config
data:
  OTEL_SERVICE_NAME: "klio-server"
  OTEL_RESOURCE_DETECTORS: "telemetry.sdk,host,os.type,process.executable.name"
  OTEL_TRACES_EXPORTER: "otlp"
  OTEL_EXPORTER_OTLP_TRACES_PROTOCOL: "grpc"
  OTEL_EXPORTER_OTLP_TRACES_ENDPOINT: "https://otel-collector:4317"
  OTEL_EXPORTER_OTLP_TRACES_COMPRESSION: "gzip"
  OTEL_EXPORTER_OTLP_TRACES_TIMEOUT: "10000"
  OTEL_EXPORTER_OTLP_TRACES_INSECURE: "false"
  OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE: "/otel/ca.crt"
  OTEL_EXPORTER_OTLP_TRACES_CLIENT_CERTIFICATE: "/otel/tls.crt"
  OTEL_EXPORTER_OTLP_TRACES_CLIENT_KEY: "/otel/tls.key"
  OTEL_METRICS_EXPORTER: "otlp"
  OTEL_METRIC_EXPORT_INTERVAL: "60000"
  OTEL_EXPORTER_OTLP_METRICS_PROTOCOL: "grpc"
  OTEL_EXPORTER_OTLP_METRICS_ENDPOINT: "https://otel-collector:4317"
  OTEL_EXPORTER_OTLP_METRICS_TIMEOUT: "60000"
  OTEL_EXPORTER_OTLP_METRICS_INSECURE: "false"
  OTEL_EXPORTER_OTLP_METRICS_CERTIFICATE: "/otel/ca.crt"
  OTEL_EXPORTER_OTLP_METRICS_CLIENT_CERTIFICATE: "/otel/tls.crt"
  OTEL_EXPORTER_OTLP_METRICS_CLIENT_KEY: "/otel/tls.key"
---
apiVersion: klio.cnpg.io/v1alpha1
kind: Server
metadata:
  name: my-klio-server
spec:
  # ... other configuration ...
  template:
    spec:
      containers:
        - name: server
          envFrom:
            - configMapRef:
                name: klio-otel-config
          volumeMounts:
            - mountPath: /otel
              name: otel
      volumes:
        - name: otel
          projected:
            sources:
              - secret:
                  name: otel-collector-tls
                  items:
                    - key: ca.crt
                      path: ca.crt
              - secret:
                  name: otel-client-cert
                  items:
                    - key: tls.crt
                      path: tls.crt
                    - key: tls.key
                      path: tls.key
```

### Klio plugins with OpenTelemetry

When deploying Klio as a CNPG Cluster plugin, configure OpenTelemetry by
specifying the necessary environment variables in the `containers` section of
the `PluginConfiguration` spec. The available container names are:

- `klio-plugin`: Main plugin sidecar for backup management
- `klio-restore`: Restore operations sidecar

Create a `ConfigMap` for the shared OpenTelemetry configuration:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cluster-klio-otel-config
data:
  OTEL_RESOURCE_DETECTORS: "telemetry.sdk,host,os.type,process.executable.name"
  OTEL_TRACES_EXPORTER: "otlp"
  OTEL_METRICS_EXPORTER: "otlp"
  OTEL_EXPORTER_OTLP_PROTOCOL: "grpc"
  OTEL_EXPORTER_OTLP_ENDPOINT: "https://otel-collector:4317"
  OTEL_EXPORTER_OTLP_COMPRESSION: "gzip"
  OTEL_EXPORTER_OTLP_TIMEOUT: "10000"
  OTEL_EXPORTER_OTLP_INSECURE: "false"
  OTEL_EXPORTER_OTLP_CERTIFICATE: "/projected/ca.crt"
  OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE: "/projected/tls.crt"
  OTEL_EXPORTER_OTLP_CLIENT_KEY: "/projected/tls.key"
```

Configure the `PluginConfiguration` to inject the environment variables into
each sidecar container:

```yaml
apiVersion: klio.cnpg.io/v1alpha1
kind: PluginConfiguration
metadata:
  name: client-config-cluster-example
spec:
  serverAddress: klio.default
  clientSecretName: cluster-example-klio-user
  serverSecretName: klio-server-tls
  clusterName: cluster-example
  containers:
    - name: klio-plugin
      env:
        - name: OTEL_SERVICE_NAME
          value: "klio-plugin"
      envFrom:
        - configMapRef:
            name: cluster-klio-otel-config
    - name: klio-restore
      env:
        - name: OTEL_SERVICE_NAME
          value: "klio-restore"
      envFrom:
        - configMapRef:
            name: cluster-klio-otel-config
```

Mount the OpenTelemetry certificates using the Cluster's `projectedVolumeTemplate`.
The projected volume is mounted at `/projected/` and is accessible to all
sidecar containers:

```yaml
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: cluster-example
spec:
  instances: 3

  projectedVolumeTemplate:
    sources:
      - secret:
          name: otel-collector-tls
          items:
            - key: ca.crt
              path: ca.crt
      - secret:
          name: otel-client-cert
          items:
            - key: tls.crt
              path: tls.crt
            - key: tls.key
              path: tls.key

  plugins:
    - name: klio.cnpg.io
      enabled: true
      parameters:
        pluginConfigurationRef: client-config-cluster-example

  storage:
    size: 10Gi
```

### Klio operator with OpenTelemetry

The operator bridges the controller-runtime Prometheus metrics
registry to OTLP and adds Go runtime and host instrumentation.
When no `OTEL_*` environment variables are present, a no-op
meter provider is installed and the operator runs without
telemetry overhead.

The existing Prometheus `/metrics` endpoint remains available
for pull-based scraping regardless of whether OTLP export is
enabled.

To enable OTLP export, set `OTEL_*` variables through the Helm
chart's `controllerManager.manager.env` value:

```yaml
controllerManager:
  manager:
    env:
      OTEL_SERVICE_NAME: "klio-operator"
      OTEL_EXPORTER_OTLP_ENDPOINT: "http://otel-collector:4318"
      OTEL_EXPORTER_OTLP_PROTOCOL: "http/protobuf"  # or "grpc"
```

The Helm chart automatically injects `POD_NAME`,
`NAMESPACE_NAME`, and `CONTAINER_NAME` via the Kubernetes
downward API, so the corresponding `k8s.*` resource attributes
are populated without additional configuration.
