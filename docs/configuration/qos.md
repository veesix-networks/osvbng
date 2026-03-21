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

### Monitoring

View active CAKE scheduler state via the show API:

```bash
curl http://localhost:8080/api/show/qos.scheduler
```

Modify or disable a scheduler at runtime via the operational API:

```bash
# Change rate
curl -X POST http://localhost:8080/api/oper/qos.scheduler.set \
  -d '{"sw_if_index": 5, "rate_kbps": 200000, "tin_mode": "diffserv4"}'

# Disable
curl -X POST http://localhost:8080/api/oper/qos.scheduler.set \
  -d '{"sw_if_index": 5, "disable": true}'
```

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
