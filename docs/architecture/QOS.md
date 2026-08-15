# QoS Architecture

osvbng can enforce per-subscriber QoS in the dataplane using either a policer or a CAKE scheduler, depending on the policy configuration. QoS is managed automatically through the subscriber session lifecycle.

Both policers and CAKE scheduling are software-based and run on VPP worker threads. Enabling QoS will consume additional CPU cycles per packet. The impact scales with subscriber count and traffic volume, so capacity planning should account for QoS overhead.

When configured, the **ingress** direction (upload, subscriber to internet) uses a policer. The **egress** direction (download, internet to subscriber) uses a policer by default, but can be switched to CAKE scheduling by adding a `scheduler` block to the egress policy.

## Policer

Token bucket policer instantiated per subscriber per direction. Each subscriber gets independent rate limiting state. No queuing or flow isolation.

## CAKE Scheduler

The CAKE (Common Applications Kept Enhanced) scheduler provides egress-only traffic shaping with per-flow fair queuing, active queue management, and DSCP-aware tin classification. It runs in the dataplane via a custom VPP plugin (`osvbng_qos_sched`).

Where policers simply drop excess traffic, CAKE queues and paces it. Each traffic flow (an upload, a video call, a game session, a download) gets its own queue. CAKE serves these queues round-robin, so a large bulk transfer cannot starve a small latency-sensitive flow. When a flow sends more than its fair share, CAKE applies backpressure rather than just dropping packets. The result is low latency under load, fair sharing of the subscriber's bandwidth, and no wasted link capacity when only one flow is active.

### How It Works

When a subscriber session activates and the egress policy has a `scheduler` block:

1. osvbng attaches a CAKE instance to the subscriber at the configured rate
2. The ingress direction gets a standard policer (if configured)
3. The egress direction is shaped by CAKE instead of policed

When the session is released, the scheduler is detached.

### Rate Selection

The shaping rate is determined in order of priority:

1. Service group `download-rate` override (if set)
2. Egress policy `cir` value

This allows a single CAKE policy definition to be shared across service groups with different speeds.

### Tin Modes

Tin modes control how traffic is classified into priority queues based on DSCP:

| Mode | Tins | Use Case |
|------|------|----------|
| `besteffort` | 1 | All traffic in a single queue |
| `diffserv3` | 3 | Bulk, Best Effort, Voice |
| `diffserv4` | 4 | Bulk, Best Effort, Video, Voice |
| `diffserv8` | 8 | Full 8-tin DSCP classification |

## Hierarchical QoS

Above the per-subscriber schedulers sit up to two aggregate shaping tiers,
forming a three-level hierarchy:

```
port aggregate            one per physical/bond interface
  └─ S-VLAN aggregate     one per configured outer-tag or tag range (optional)
       └─ subscriber CAKE scheduler   one per session
```

### How each level schedules

**Subscriber scheduler** — the CAKE instance described above: DSCP tins →
DRR flow queues → COBALT AQM, gated by its own virtual-time shaper. Real
queues live only at this level.

**Aggregates** — an aggregate holds no queues and no child lists. It is a
shaper gate (a lockless virtual-time clock shared by all workers) plus
weighted-DRR arbitration state. Arbitration is *child-driven*: every child
carries its own deficit and refuses itself when it is spent, so nothing on
the packet path ever walks a list of children. What a parent publishes is
only the sum of its active children's weights (`active_weight`), which moves
on activation transitions, not per packet.

A child's DRR share is its **effective weight**: derived from its configured
rate, multiplied by the operator `weight` (1-256). Weight multiplies rather
than replaces the rate-derived share so mixed configuration stays
meaningful. `active_weight / effective_weight` is the fraction of the parent
a child is entitled to while contended.

Two per-child counters distinguish *why* a dispatch was deferred:

- `drr_blocked` — the child spent its share of the tier above; siblings hold
  the capacity.
- `parent_blocked` — the parent chain itself was at its configured rate.

Activation tracks traffic, not configuration: a scheduler deactivates the
moment its queues drain and re-activates on the next packet, so
`active_children` moves with load.

### Identity and attachment

A port aggregate and every S-VLAN aggregate beneath it are all addressed by
the **port's** `sw_if_index`; the composite key `(sw_if_index, level,
svlan_id)` is what distinguishes them, on the API and in every show view.

Attachment is automatic. When a scheduler is created the dataplane walks the
session interface's `sup_sw_if_index` chain to the physical port, notes the
outer tag along the way, and attaches the scheduler to the S-VLAN aggregate
covering that tag, or directly to the port when none does. There is no bind
API and no member lists to maintain; buffer admission is charged against the
subscriber's overhead-adjusted packet length up the whole chain.

### How the control plane reads the hierarchy

The dataplane keeps no child lists, so hierarchy views are joined
control-plane-side:

- The scheduler v2 dump (`osvbng_cake_sched_v2_dump`, plugin API >= 3.1.0)
  reports each scheduler's parent as the same composite key the aggregate
  dump uses, plus its DRR state and throughput counters. It also accepts a
  parent filter, so `show qos aggregate detail` costs one targeted dump per
  tier instead of a full walk.
- Session identity comes from the subscriber component's in-memory
  `sw_if_index -> session-id` index, maintained on session persist/delete.
  It is best-effort: a row can lose its session id if the session tears down
  mid-dump.

### Capability negotiation

Message CRCs are checked at runtime, so the plugin only ever *adds*
messages. The control plane discovers what the dataplane supports via
`osvbng_cake_capabilities` (feature bits for the S-VLAN tier and weighted
DRR) and, for the scheduler v2 dump, by sending it once and treating the
message's absence as the answer — a deliberate choice, because adding a
feature bit would have changed the capabilities reply's CRC and broken older
control planes. Against a v1-only dataplane every view still renders; the
v2-only fields read zero and membership carries an explanatory note.

## Related Documentation

- [QoS Configuration](../configuration/qos.md) - Policy, scheduler and aggregate configuration reference
- [Service Groups](../configuration/service-groups.md) - How QoS policies are applied to subscribers
