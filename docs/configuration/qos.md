# QoS Policies <span class="version-badge">v0.2.0</span>

QoS policies define per-subscriber rate limiting and traffic shaping. Each policy is a named template instantiated per subscriber at session activation.

CAKE scheduling is only supported on the subscriber **egress** direction (download, internet to subscriber). The **ingress** direction (upload, subscriber to internet) always uses a policer.

Policies are defined under the top-level `qos-policies` key and referenced by name from [service groups](service-groups.md). For architectural details, see [QoS Architecture](../architecture/QOS.md).

## Policy Settings

| Field | Type | Description | Default |
|-------|------|-------------|---------|
| `cir` | uint32 | Committed information rate (kbps) | required |
| `eir` | uint32 | Excess information rate (kbps) | equal to `cir` |
| `cbs` | uint64 | Committed burst size (bytes) | `cir * 1000 / 8` |
| `ebs` | uint64 | Excess burst size (bytes) | equal to `cbs` |
| `conform` | [Action](#actions) | Action for conforming traffic | required (policer-only) |
| `exceed` | [Action](#actions) | Action for exceeding traffic | required (policer-only) |
| `violate` | [Action](#actions) | Action for violating traffic | required (policer-only) |
| `scheduler` | [Scheduler](#cake-scheduler) | CAKE scheduler config | optional |

All rates are in **kilobits per second**. For example, `cir: 100000` = 100 Mbps.

When no `scheduler` block is present, the policy operates as a pure policer using the **2-rate 3-colour** (2R3C) model defined in RFC 2698. Traffic is classified into one of three colours based on instantaneous rate against the CIR and EIR token buckets, and the configured action is applied.

## CAKE Scheduler <span class="version-badge">v0.6.0</span>

Adding a `scheduler` block to a policy switches the egress direction from policer to CAKE-based shaping with fair queuing and AQM.

### Scheduler Settings

| Field | Type | Description | Default |
|-------|------|-------------|---------|
| `tin-mode` | string | DSCP-to-tin classification mode | `besteffort` |
| `weight` | uint32 | DRR weight multiplier under an aggregate, 1-256. Multiplies the share the subscriber's rate already earns; see [Hierarchical QoS](#hierarchical-qos-aggregates) | `1` |

### Tin Modes

| Value | Tins | Description |
|-------|------|-------------|
| `besteffort` | 1 | Single tin, all traffic treated equally |
| `diffserv3` | 3 | Bulk, Best Effort, Voice |
| `diffserv4` | 4 | Bulk, Best Effort, Video, Voice (recommended) |
| `diffserv8` | 8 | Full 8-tin DSCP classification |

### Example: CAKE Egress Shaping

```yaml
qos-policies:
  cake-100m:
    cir: 100000
    scheduler:
      tin-mode: diffserv4

service-groups:
  residential:
    qos:
      egress-policy: cake-100m
```

This creates a 100 Mbps CAKE shaper with 4-tin DiffServ classification on each subscriber's egress. No policer action fields are needed when using the scheduler.

### Example: CAKE Egress with Ingress Policer

```yaml
qos-policies:
  upload-50m:
    cir: 50000
    conform:
      action: transmit
    exceed:
      action: drop
    violate:
      action: drop

  download-100m-shaped:
    cir: 100000
    scheduler:
      tin-mode: diffserv4

service-groups:
  residential:
    qos:
      ingress-policy: upload-50m
      egress-policy: download-100m-shaped
```

### Example: Per-Service-Group Rate Override

The service group's `download-rate` field overrides the policy's `cir` for the scheduler rate. This lets you share a single CAKE policy across service groups with different speeds.

```yaml
qos-policies:
  cake-shaped:
    cir: 100000
    scheduler:
      tin-mode: diffserv4

service-groups:
  residential-100m:
    qos:
      egress-policy: cake-shaped

  residential-500m:
    qos:
      egress-policy: cake-shaped
      download-rate: 500000
```

## Hierarchical QoS (Aggregates) <span class="version-badge">v0.9.0</span>

Above the per-subscriber schedulers, the dataplane can shape two aggregate
tiers: an **S-VLAN aggregate** (one shaper for every subscriber behind an
outer tag or tag range) under a **port aggregate** (one shaper for the whole
physical or bond interface). Contention at each tier is arbitrated by
weighted deficit round robin across the tier's active children. See
[QoS Architecture](../architecture/QOS.md) for how the hierarchy schedules.

Aggregates are defined under the top-level `qos-aggregates` key, one named
entry per tier instance:

```yaml
qos-aggregates:
  port:
    interface: eth1        # physical or bond interface
    rate: 8000             # kbps
  svlan-100:
    interface: eth1
    svlans: ["100"]        # single tag
    rate: 6000
  svlan-rest:
    interface: eth1
    svlans: ["200-300"]    # tag range: one shaper for every tag in it
    rate: 3000
    weight: 4
    burst-ms: 50
```

### Aggregate Settings

| Field | Type | Description | Default |
|-------|------|-------------|---------|
| `interface` | string | Physical or bond interface, both levels | required |
| `svlans` | []string | Outer tags this aggregate shapes: a tag (`"100"`) or a range (`"200-300"`). Omit entirely for a port aggregate | port level |
| `rate` | uint32 | Shaping rate (kbps) | required |
| `weight` | uint32 | DRR weight multiplier under the parent, 1-256 | `1` |
| `burst-ms` | uint32 | Idle credit ceiling, 10-150 ms | `10` |
| `buffer-limit` | uint32 | Max buffered bytes | derived from rate |

### Requirements Per Level

**Port aggregate** — keyed by the physical/bond interface, no `svlans` list.
At most one per interface. Must exist before (or be committed together with)
any S-VLAN aggregate on the same interface.

**S-VLAN aggregate** — requires a port aggregate on the same interface. Each
entry takes a single tag or one `a-b` range (comma lists are rejected — use
separate entries); tag sets of all S-VLAN aggregates on one port must be
disjoint; tags run 1-4095. An S-VLAN's `rate` may not exceed its port's.

**Subscriber scheduler** — enabled per session by the egress policy's
`scheduler` block (see above); there is no per-aggregate member list to
maintain. Attachment is automatic: the dataplane walks the session
interface's parent chain to the physical port and, when the session's outer
tag is covered by an S-VLAN aggregate, attaches it there, otherwise directly
to the port aggregate. The scheduler's DRR share is derived from its rate,
multiplied by the policy's `weight`.

Rate and weight changes to an existing aggregate are applied in place; a
change to `interface` or the tag set recreates the aggregate (dropping and
re-attaching its members).

### Monitoring

Show commands (each also available at `/api/show/qos/...` and, with
`| json`, as raw JSON in the CLI). Session IDs are UUIDs for every access
type, exactly as `show subscriber sessions` reports them:

```text
show qos scheduler [--interface X]
    Every subscriber scheduler: rate, weight, DRR state, throughput,
    drops, buffer usage, session id.

show qos scheduler session --session-id <session-uuid>
show qos scheduler session --acct-session-id <aaa-acct-session-id>
show qos scheduler session --interface <access-if> --outer-vlan N [--inner-vlan M]
    One subscriber's full shaping chain: session identity, scheduler with
    per-tin stats, and the S-VLAN / port aggregates above it. An ambiguous
    interface+VLAN lookup returns the matching candidates instead.

show qos scheduler detail --interface <session-if>
    The same view addressed by the scheduler's interface (name or
    sw_if_index) rather than by subscriber identity.

show qos aggregate [--interface X] [--level port|svlan] [--svlan N]
    Both aggregate tiers; --svlan matches the aggregate whose tag range
    covers N.

show qos aggregate detail --interface <port> [--svlan N]
    The whole hierarchy under one port as a tree: the port aggregate, its
    S-VLAN aggregates, and every member scheduler with stats and session id.
```

Any command runs non-interactively with `osvbngcli -c "<command>"`, which
prints the result with no banner and exits non-zero on failure — the form
scripts and the test suites use.

In the CLI, the two list views render compactly. `show qos scheduler` is
one line per scheduler:

```text
SW_IF  INTERFACE    SESSION                               RATE  MODE  W(EFF)   PARENT      ST  TX PKTS/BYTES  DROP  Q   BUF  BLK D/P
-----  -----------  ------------------------------------  ----  ----  -------  ----------  --  -------------  ----  --  ---  -----------
7      ipoe0.100.1  9f8be1a2-77c1-4a83-9e0f-0d5a2f6b3c11  2M    ds4   1(250K)  sv100@eth1  A   18.2M/25.9G    1.2K  14  5%   88.2K/12.1K
```

MODE abbreviates the tin mode (`be`, `ds3`, `ds4`, `ds8`); `W(EFF)` is the
configured weight with the effective DRR weight; `ST` is `A` while the
scheduler is active in its parent's arbitration; `BUF` is buffer usage as a
percentage of the limit; `BLK D/P` is `drr_blocked`/`parent_blocked`.
Values are SI-scaled. `show qos aggregate` renders each port's hierarchy as
a tree:

```text
eth1  port  8M  ·  3 active / W 1.8M  ·  buf 1.2M/16.8M (7%)
│       shaped 88.2M pkts / 112.4 GB   backpressure 122   parent-blk 904.2K
├─ svlan 100  6M  w1 (eff 750K)   2 active / W 500K   buf 96.3K/1M (9%)
│       shaped 40.1M pkts / 51.2 GB   backpressure 31   blk drr 421.9K par 88.1K
└─ svlan 200-300  3M  w1 (eff 375K)   2 active / W 250K   buf 0/1M (0%)
        shaped 9M pkts / 11.1 GB   backpressure 0   blk drr 10.2K par 4.4K
```

The full field set is always available with `| json`.

Modify or disable a scheduler at runtime via the operational API:

```bash
# Change rate
curl -X POST http://localhost:8080/api/oper/qos.scheduler.set \
  -d '{"sw_if_index": 5, "rate_kbps": 200000, "tin_mode": "diffserv4"}'

# Disable
curl -X POST http://localhost:8080/api/oper/qos.scheduler.set \
  -d '{"sw_if_index": 5, "disable": true}'
```

### Prometheus Metrics

Both QoS show paths are polled by the telemetry SDK every 10 seconds and
exported by the `exporter.prometheus` plugin. Families and labels:

| Family | Type | Labels |
|--------|------|--------|
| `osvbng_qos_scheduler_{rate_kbps,tin_count,weight,effective_weight,buffer_usage,buffer_limit,queued_buffers}` | gauge | `sw_if_index`, `interface`, `tin_mode` |
| `osvbng_qos_scheduler_{enqueued,dequeued}_{packets,bytes}`, `_dropped_packets`, `_drr_blocked`, `_parent_blocked` | counter | `sw_if_index`, `interface`, `tin_mode` |
| `osvbng_qos_scheduler_tin_{packets,bytes,drops,ecn_marks}` | counter | scheduler labels + `tin` |
| `osvbng_qos_scheduler_tin_{sparse_flows,bulk_flows,flow_count,peak_delay_us,avg_delay_us}` | gauge | scheduler labels + `tin` |
| `osvbng_qos_aggregate_{rate_kbps,weight,effective_weight,burst_ms,buffer_usage,buffer_limit,active_weight,active_children}` | gauge | `sw_if_index`, `interface`, `level`, `svlan_id`, `svlan_id_end` |
| `osvbng_qos_aggregate_{shaped_packets,shaped_bytes,backpressure,drr_blocked,parent_blocked}` | counter | aggregate labels |

Notes:

- Scheduler metrics are per subscriber (keyed by the session interface).
  Each metric family is capped at 10,000 series; overflow is dropped and
  counted in `osvbng_telemetry_cardinality_drops_total`. Session identity is
  deliberately **not** a label — correlate `sw_if_index`/`interface` through
  the show API instead.
- `tin_peak_delay_us` / `tin_avg_delay_us` read zero until the dataplane
  computes per-tin sojourn delay.

### Dataplane Version Requirements

The control plane probes the dataplane and degrades rather than failing:

| Feature | Needs QoS plugin API |
|---------|----------------------|
| Per-subscriber CAKE scheduler, per-tin stats | any |
| Aggregates (port + S-VLAN), scheduler `weight` | >= 3.0.0 |
| Scheduler DRR/parent state, throughput counters, `show qos aggregate detail` membership, per-tin `flow_count` | >= 3.1.0 |

Against an older dataplane the affected fields read zero and the aggregate
detail view notes that membership is unavailable.

## Actions

Each action block specifies what to do with traffic in that colour class. Only required for policer-mode policies (no `scheduler` block).

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `action` | string | `transmit`, `drop`, or `mark-and-transmit` | `transmit` |
| `dscp` | uint8 | DSCP value to mark (only used with `mark-and-transmit`) | `46` |

## Policer Usage

QoS policies without a `scheduler` block are applied as VPP policers. A service group references policy names for ingress (upload) and egress (download) directions independently.

```yaml
qos-policies:
  100m-policer:
    cir: 100000
    conform:
      action: transmit
    exceed:
      action: drop
    violate:
      action: drop

service-groups:
  residential:
    qos:
      ingress-policy: 100m-policer
      egress-policy: 100m-policer
```

When a subscriber session activates, the referenced policies are instantiated as VPP policers and attached to the subscriber's sub-interface. When the session is released, the policers are detached and deleted.

## Asymmetric Rates

Use different policies for upload and download to create asymmetric speed profiles.

```yaml
qos-policies:
  upload-50m:
    cir: 50000
    conform:
      action: transmit
    exceed:
      action: drop
    violate:
      action: drop

  download-200m:
    cir: 200000
    conform:
      action: transmit
    exceed:
      action: drop
    violate:
      action: drop

service-groups:
  residential:
    qos:
      ingress-policy: upload-50m
      egress-policy: download-200m
```

## DSCP Marking

Use `mark-and-transmit` to remark excess traffic instead of dropping it.

```yaml
qos-policies:
  business-with-remarking:
    cir: 100000
    eir: 200000
    conform:
      action: transmit
    exceed:
      action: mark-and-transmit
      dscp: 0
    violate:
      action: drop
```

In this example, traffic up to 100 Mbps is forwarded unchanged, traffic between 100-200 Mbps is remarked to DSCP 0 (best effort), and traffic above 200 Mbps is dropped.

## AAA Override

AAA can override QoS policy names per subscriber by returning `qos.ingress-policy` and `qos.egress-policy` attributes. See [service groups](service-groups.md#aaa-attributes) for the full list of overridable attributes.
