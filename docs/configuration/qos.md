# QoS Policies

QoS policies define per-subscriber rate limiting using VPP's native policer engine. Each policy is a named template that gets instantiated as an independent policer per subscriber at session activation, giving each subscriber its own token bucket state.

Policies are defined under the top-level `qos-policies` key and referenced by name from [service groups](service-groups.md).

## Policy Settings

| Field | Type | Description | Default |
|-------|------|-------------|---------|
| `cir` | uint32 | Committed information rate (kbps) | required |
| `eir` | uint32 | Excess information rate (kbps) | equal to `cir` |
| `cbs` | uint64 | Committed burst size (bytes) | `cir * 1000 / 8` |
| `ebs` | uint64 | Excess burst size (bytes) | equal to `cbs` |
| `conform` | [Action](#actions) | Action for conforming traffic | required |
| `exceed` | [Action](#actions) | Action for exceeding traffic | required |
| `violate` | [Action](#actions) | Action for violating traffic | required |

All rates are in **kilobits per second**. For example, `cir: 100000` = 100 Mbps.

The policer uses the **2-rate 3-colour** (2R3C) model defined in RFC 2698. Traffic is classified into one of three colours based on instantaneous rate against the CIR and EIR token buckets, and the configured action is applied.

## Actions

Each action block specifies what to do with traffic in that colour class.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `action` | string | `transmit`, `drop`, or `mark-and-transmit` | `transmit` |
| `dscp` | uint8 | DSCP value to mark (only used with `mark-and-transmit`) | `46` |

## Usage

QoS policies are applied to subscribers through service groups. A service group references policy names for ingress (upload) and egress (download) directions independently.

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
