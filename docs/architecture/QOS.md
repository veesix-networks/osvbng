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

## Related Documentation

- [QoS Configuration](../configuration/qos.md) - Policy and scheduler configuration reference
- [Service Groups](../configuration/service-groups.md) - How QoS policies are applied to subscribers
