# Telemetry SDK

osvbng exposes a typed in-memory metric registry as `pkg/telemetry`. Components and plugin authors register typed counters, gauges, and histograms; consumers (Prometheus exporter, gRPC streaming exporter, gNMI gateway) read snapshots or subscribe to updates. Atomic primitives, no JSON, no reflection on the hot path.

## Why

The earlier observability pipeline pushed every show-handler response through a JSON-marshalled cache, then read it back through reflection per Prometheus scrape. At 48k subscribers this burned 117% CPU at idle. The telemetry SDK replaces that pipeline for *metric* state with a registry that:

- Stores counters and integer values in `atomic.Uint64`; float gauges and histogram sums use a `math.Float64bits` CAS loop
- Pre-resolves label tuples to per-series handles; the hot emit path is a single atomic add
- Caps cardinality at registration time and rejects unbounded labels by default
- Drives streaming subscribers from a single tick goroutine via a per-metric dirty flag, not per-emit fan-out

## Architecture

```
True push                      Cheap pull (shared mem)        Expensive pull (RPC/shell)
─────────                      ───────────────────────        ──────────────────────────
session lifecycle              VPP stats segment              FRR (vtysh)
AAA req/resp                                                  RADIUS server probes
HA state changes                                              BGP/OSPF/ISIS/MPLS
interface UP/DOWN
       │                              │                              │
       │              ┌───────────────┴───────────────┐              │
       │              │ Bounded poller pool           │              │
       │              │  cheap-pull tier (10s)        │              │
       │              │  expensive-pull tier (30s)    │              │
       │              │  delta-suppression on emit    │              │
       │              └───────────────┬───────────────┘              │
       ▼                              ▼                              ▼
       ┌───────────────────────────────────────────────────────────────┐
       │ pkg/telemetry registry                                        │
       │  - Counter / Gauge / Histogram atomic primitives              │
       │  - Per-series label hash → atomic primitive                   │
       │  - Cardinality budget enforced at registration                │
       │  - Per-metric atomic.Bool dirty flag                          │
       └───────────────────────────┬───────────────────────────────────┘
                                   │
                ┌──────────────────┼──────────────────┐
                ▼                  ▼                  ▼
         Prometheus exporter   gRPC streaming    gNMI gateway
         (snapshot at /metrics) (server-stream)  (json_ietf_val)
```

## Hot-path cost model

Every primitive operation on a pre-resolved handle is a single `atomic` instruction. Specifically, on AMD Ryzen 9 9900X (single thread, no other load):

| Operation | Time per emit | Allocations |
|-----------|---------------|-------------|
| `Counter.Inc()` (resolved handle, no subscribers) | 3.77 ns | 0 |
| `Counter.Inc()` (resolved handle, 1 subscriber) | 3.76 ns | 0 |
| `Counter.Inc("v")` (variadic, existing tuple) | 9.94 ns | 0 |
| `Counter.Inc("v")` (variadic, unknown tuple → drop) | 8.05 ns | 0 |
| `Gauge.Set(v)` (resolved) | 3.96 ns | 0 |
| `Gauge.Add(v)` (resolved, float CAS loop) | 4.04 ns | 0 |
| `Histogram.Observe(v)` (resolved, 8 buckets) | 11.40 ns | 0 |
| `Counter.Inc()` on tombstone (over-budget tuple) | 3.64 ns | 0 |

The "with subscriber" and "without subscriber" emit benchmarks are statistically identical. The dirty-flag CAS fires at most once per tick window per metric, not per emit. See [Subscribe model](#subscribe-model) below.

Snapshot:

| Operation | Time per snapshot | Allocations |
|-----------|-------------------|-------------|
| `AppendSnapshot` of 1000 counter series, dst pre-sized | 7.86 µs | 0 |
| `AppendSnapshot` of 50 histogram series (8 buckets each) | 1.54 µs | 50 (one bucket slice per series) |

Histogram snapshots allocate one `[]BucketSample` per series per snapshot. This is by design: histogram bucket counts vary per metric and a caller-owned scratch design adds API complexity for negligible benefit. Counter and gauge snapshots are allocation-free.

## Hash and series resolution

Label tuples map to series via FNV-1a over the raw label-value bytes. Each value is hashed byte-by-byte with a `0xFF` delimiter inserted between values to disambiguate tuples like `("ab","c")` from `("a","bc")`. There is no `strings.Join` or `[]byte(s)` conversion. The hot path allocates nothing.

The per-metric `sync.Map[uint64, *seriesEntry]` is read lock-free in the common case. Hash collisions (extremely rare) are verified by per-element string compare on the stored `[]string`.

Cold-create (first call to `WithLabelValues` for a tuple) uses `LoadOrStore`. Multiple goroutines may construct candidate `seriesEntry` values under contention; only the winner increments the cardinality counter and stores label-pair metadata. Losers GC their candidate. The hot read path never enters this code.

## Cardinality budget

Two layers protect operators from a single bad metric registration:

**Default reject list.** Registration with any of these label names returns `ErrUnboundedLabel` unless the metric sets `StreamingOnly: true`:

```
session_id, subscriber_id, session, subscriber,
auth_session_id, acct_session_id,
ip, ipv4, ipv6, mac, calling_station_id,
username, hostname,
circuit_id, remote_id, agent_circuit_id, agent_remote_id,
nas_port_id
```

The list can be replaced via `telemetry.SetUnboundedLabels([]string{...})` before any registration.

**Per-metric series cap.** `MaxSeriesPerMetric` (default 10000) bounds the number of distinct label tuples per metric. When exceeded, further `WithLabelValues` calls return a per-metric **singleton tombstone handle**. The tombstone's `Inc`/`Set`/`Observe` increment a per-metric drop counter rather than the per-series value. Operators see the cardinality cliff as monotonic growth on `osvbng_telemetry_cardinality_drops_total{metric}`, not as a workload speedup.

The variadic emit path (`counter.Inc(labelValues...)` without a pre-resolved handle) is **lookup-or-drop**, not lookup-or-create. An unknown tuple bumps `osvbng_telemetry_unknown_series_emits_total{metric}` and returns. Only `WithLabelValues` and `ResolveLabelValues` create new series, and both are intended for cold paths (component start, session establishment).

## Subscribe model

`Subscribe(opts)` returns a `Subscription` whose channel emits `Update` records driven by a single registry-internal tick goroutine. The tick walks every metric with the dirty flag set, snapshots its current series, dispatches to each matching subscriber, then clears the flag.

The dirty flag is a per-metric `atomic.Bool`. Emit logic:

1. If subscriber count is zero, do not touch the flag at all. Hot path is identical to the no-subscribe case.
2. If subscribers exist and the flag is already true, do not touch it (the tick will pick it up).
3. If subscribers exist and the flag is false, attempt one `CompareAndSwap(false, true)`. Only the first emitter between ticks pays this; subsequent emitters in the same tick window see the flag already set.

This keeps the steady-state hot path at one atomic instruction regardless of subscriber count. The trade-off: subscriber latency is bounded by the tick cadence (default 1 second). Set via `telemetry.SetTickInterval(d)` before the first Subscribe; calls after the tick goroutine has started have no effect on the running goroutine.

Per-subscriber bounded channels with **drop-only** overflow. There is no `Block` policy: a slow subscriber must not be able to stall the single tick goroutine and starve every other subscriber. Per-subscription drop counts surface as `osvbng_telemetry_subscription_drops_total{subscriber_id}`.

The first Subscribe lazily starts the tick goroutine; the last Unsubscribe stops it. Subsequent Subscribes restart it. Buffered updates after Unsubscribe remain readable until drained.

## Internal observability metrics

The registry surfaces the following metrics about its own state during `AppendSnapshot`:

| Metric | Type | Labels | Purpose |
|--------|------|--------|---------|
| `osvbng_telemetry_metrics_total` | gauge | (none) | number of registered metrics |
| `osvbng_telemetry_series_total` | gauge | `metric` | series count per metric |
| `osvbng_telemetry_subscriptions_total` | gauge | (none) | active subscribers |
| `osvbng_telemetry_subscription_drops_total` | counter | `subscriber_id` | drops per subscription |
| `osvbng_telemetry_cardinality_drops_total` | counter | `metric` | tombstone-path drops (over-budget tuples) |
| `osvbng_telemetry_unknown_series_emits_total` | counter | `metric` | variadic emits to non-existent series |
| `osvbng_telemetry_stale_handle_emits_total` | counter | `metric` | emits via handles whose series were unregistered |
| `osvbng_telemetry_registration_errors_total` | counter | `reason` | failed registrations |

These run through the same snapshot path as application metrics and surface through whatever exporters consume the registry.

## Lifecycle

The registry is a `Registry` type with a package-level default and a `NewRegistry()` constructor. Production code uses package-level functions (`telemetry.RegisterCounter`, `telemetry.AppendSnapshot`, `telemetry.Subscribe`); tests construct isolated registries to avoid cross-test state leakage under `t.Parallel()`.

`telemetry.Shutdown(ctx)` stops the tick goroutine and waits for it to exit. `osvbngd` calls this during graceful shutdown.

Series-level cleanup is supported via `handle.UnregisterSeries(labelValues...)`, which removes the series from the metric's lookup map and decrements the series count. Retained handles for the removed tuple become "stale": subsequent emits do not panic and do not write to the freed primitive; they bump `osvbng_telemetry_stale_handle_emits_total`. This is the cleanup hook for high-churn deployments (per-session metrics that come and go).

Metric-level unregister (closing an entire metric) is not currently supported. osvbng plugins are loaded once at process start; metric lifetime is process lifetime.

## HA dimension

The registry is per-process. In dual-active SRG deployments, each peer publishes only the metrics for the SRGs it owns; metrics carry an `srg` label so subscribers can demux. There is no metric replication across peers. Clients reconnect or subscribe to both peers and aggregate client-side.

## Integration points

The poller pool, Prometheus exporter rewrite, gRPC streaming exporter, FRR migration, and gNMI gateway are separate work items that build on this SDK. The exporters all consume the registry through `AppendSnapshot` or `Subscribe`; plugin authors emit through the typed primitives without knowing the wire formats.
