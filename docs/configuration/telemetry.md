# Telemetry SDK

`pkg/telemetry` is the typed in-memory metric registry that backs osvbng's exporters. Plugin authors use it to emit counters, gauges, and histograms; exporters read snapshots or subscribe to updates. See [the architecture overview](../architecture/TELEMETRY.md) for the design rationale.

This page is the SDK reference. It does not describe a YAML config block; the SDK has no operator-facing config in this release.

## Registering metrics

Metrics are registered against either the package-level default registry (production code) or an isolated `Registry` constructed via `telemetry.NewRegistry()` (tests).

```go
import "github.com/veesix-networks/osvbng/pkg/telemetry"

// Counter
counter, err := telemetry.RegisterCounter(telemetry.CounterOpts{
    Name:   "osvbng_aaa_requests_total",
    Help:   "Total AAA requests by access type and VRF.",
    Labels: []string{"access_type", "vrf"},
})

// Gauge
gauge, err := telemetry.RegisterGauge(telemetry.GaugeOpts{
    Name:   "osvbng_dataplane_buffers_used",
    Help:   "VPP buffer pool utilization.",
    Labels: []string{"pool"},
})

// Histogram
hist, err := telemetry.RegisterHistogram(telemetry.HistogramOpts{
    Name:    "osvbng_session_setup_seconds",
    Help:    "Time to bring a subscriber session to active state.",
    Labels:  []string{"access_type"},
    Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 5},
})
```

Registration is idempotent: re-registering with the same name and matching schema returns the existing metric. Re-registering with a different schema returns `ErrTypeMismatch` or `ErrSchemaMismatch`.

## Resolving label tuples

Resolve a label tuple to a per-series handle via `WithLabelValues`. **Cache the handle on the cold path and reuse it for every emit.** The variadic emit methods below are intentionally slower and intentionally do not create new series.

```go
ipoeAAA := counter.WithLabelValues("ipoe", "vrf-customer-A")
pppoeAAA := counter.WithLabelValues("pppoe", "vrf-customer-A")

// Hot path
ipoeAAA.Inc()
pppoeAAA.Add(5)
```

A typical pattern: resolve handles at component start or on session establishment, store them on a struct field, emit through the field.

```go
type Component struct {
    aaaCounter *telemetry.CounterHandle
}

func (c *Component) onSessionUp(s *Session) {
    c.aaaCounter.Inc()
}
```

## Variadic emit (lookup-or-drop)

For convenience, the metric type itself exposes a variadic emit that hashes the supplied tuple and emits to the existing series:

```go
counter.Inc("ipoe", "vrf-customer-A")
gauge.Set(42, "main-pool")
hist.Observe(0.05, "ipoe")
```

These are **lookup-or-drop, never lookup-or-create**. If the tuple has not been resolved before via `WithLabelValues`, the emit is dropped and `osvbng_telemetry_unknown_series_emits_total{metric}` is incremented. This is deliberate: lazy creation on a hot path is the cardinality failure mode the SDK exists to prevent.

If you need a series that is discovered dynamically at runtime (e.g. a new VRF appears mid-session), call `WithLabelValues` once outside the hot path to register it, then use either form to emit.

## Cardinality budget

By default, registration with any of these label names returns `ErrUnboundedLabel`:

```
session_id, subscriber_id, session, subscriber,
auth_session_id, acct_session_id,
ip, ipv4, ipv6, mac, calling_station_id,
username, hostname,
circuit_id, remote_id, agent_circuit_id, agent_remote_id,
nas_port_id
```

These produce unbounded cardinality in BNG deployments. If you genuinely need per-session metrics for a streaming consumer (gRPC streaming, gNMI), set `StreamingOnly: true`:

```go
counter, err := telemetry.RegisterCounter(telemetry.CounterOpts{
    Name:          "osvbng_session_rx_bytes_total",
    Help:          "Per-session RX bytes for streaming consumers only.",
    Labels:        []string{"session_id"},
    StreamingOnly: true,
})
```

`StreamingOnly: true` excludes the metric from the default Prometheus snapshot path (`SnapshotOptions.IncludeStreamingOnly` is `false` by default). The streaming exporter explicitly opts in.

A second guard, `MaxSeriesPerMetric` (default 10000), bounds the number of distinct label tuples per metric. Once exceeded, further `WithLabelValues` calls return a per-metric singleton **tombstone handle** whose emit increments `osvbng_telemetry_cardinality_drops_total{metric}` rather than per-series state. Operators monitor this counter to spot misconfigured plugins.

To replace the unbounded label list (e.g. allowing `ipv4` for a small-deployment-only build):

```go
telemetry.SetUnboundedLabels([]string{"session_id", "mac"})
```

Call this before any registration; existing registrations are not re-validated.

## Cleanup of high-churn series

When a tracked entity goes away (a subscriber session terminates, a VRF is deleted), call `UnregisterSeries` on each metric handle to free the per-series state:

```go
counter.UnregisterSeries("ipoe", "vrf-customer-A")
```

A retained handle for the removed tuple becomes stale: subsequent emits do not panic and do not write to the freed primitive; they bump `osvbng_telemetry_stale_handle_emits_total{metric}` and return. Callers SHOULD release stale handles after `UnregisterSeries`.

This is the operationally critical hook for `StreamingOnly` per-session metrics in production. Forget to call it and the registry leaks one entry per terminated session.

## Snapshots

Exporters read the registry via `AppendSnapshot`. The caller owns the destination buffer; pre-size it once and reuse.

```go
var buf []Sample
for {
    buf = telemetry.AppendSnapshot(buf[:0], telemetry.SnapshotOptions{
        PathGlob:             "osvbng_aaa_*",
        IncludeStreamingOnly: false,
    })
    // serialize buf to the wire format
}
```

`SnapshotOptions.IncludeStreamingOnly` defaults to `false` (Prometheus-safe). The streaming exporter sets it to `true`. Counter and gauge samples are allocation-free; histogram samples allocate one bucket slice per series.

## Subscribing to updates

Streaming consumers (gRPC streaming exporter, gNMI gateway) subscribe to a stream of updates:

```go
sub := telemetry.Subscribe(telemetry.SubscribeOptions{
    PathGlob:             "osvbng_session_*",
    BufferSize:           1024,
    IncludeStreamingOnly: true,
})
defer sub.Unsubscribe()

for u := range sub.Updates() {
    // u is a Sample plus a Timestamp
}
```

Updates are delivered by a single registry-internal tick goroutine on a configurable cadence (default 1s). The tick walks every metric whose dirty flag is set since the last tick, snapshots its current series, and dispatches to each matching subscriber.

The per-subscriber channel is bounded (`BufferSize`, default 256) with **drop-only** overflow. A slow subscriber drops updates rather than blocking the tick or stalling other subscribers; drop counts surface as `osvbng_telemetry_subscription_drops_total{subscriber_id}`.

To change the tick cadence process-wide:

```go
telemetry.SetTickInterval(500 * time.Millisecond)
```

This must be set before the first `Subscribe`; calls after the tick goroutine has started do not affect the running goroutine. The setting is process-wide and should be tuned to the tightest active exporter requirement. Prometheus and gNMI consumers pull on their own schedules and are unaffected by faster ticks.

## Internal observability metrics

The registry exposes its own health through standard metrics that show up in `AppendSnapshot`:

| Metric | Type | Labels |
|--------|------|--------|
| `osvbng_telemetry_metrics_total` | gauge | (none) |
| `osvbng_telemetry_series_total` | gauge | `metric` |
| `osvbng_telemetry_subscriptions_total` | gauge | (none) |
| `osvbng_telemetry_subscription_drops_total` | counter | `subscriber_id` |
| `osvbng_telemetry_cardinality_drops_total` | counter | `metric` |
| `osvbng_telemetry_unknown_series_emits_total` | counter | `metric` |
| `osvbng_telemetry_stale_handle_emits_total` | counter | `metric` |
| `osvbng_telemetry_registration_errors_total` | counter | `reason` |

Watch `cardinality_drops_total` and `unknown_series_emits_total` to catch plugin authors using the SDK incorrectly. `stale_handle_emits_total` indicates a missing `UnregisterSeries` somewhere.

## Tests

Use `telemetry.NewRegistry()` to construct an isolated registry per test. Production code's package-level convenience functions back onto a shared default registry; using it in tests with `t.Parallel()` leaks state between tests.

```go
func TestComponent_EmitsAAACounter(t *testing.T) {
    t.Parallel()
    reg := telemetry.NewRegistry()
    c, _ := reg.RegisterCounter(telemetry.CounterOpts{
        Name:   "osvbng_aaa_requests_total",
        Labels: []string{"vrf"},
    })
    c.WithLabelValues("default").Inc()
    // assert via reg.AppendSnapshot
}
```
