# Service Groups

A service group is a reusable template that groups common subscriber service attributes together. Rather than configuring VRF, unnumbered interface, uRPF, ACLs, and QoS individually per subscriber, you define them once in a service group and reference it by name.

A subscriber group can set a `default-service-group` so all subscribers in that group inherit the same service attributes. AAA can assign a different service group per subscriber, or override individual fields.

## Attribute Priority

When a subscriber authenticates, their effective attributes are determined by merging three layers (highest priority first):

1. **Per-subscriber AAA attributes** - field-level overrides returned by AAA (e.g. `vrf`, `unnumbered`, `qos.download-rate`)
2. **AAA service group** - if AAA returns a `service-group` attribute, that named group is applied
3. **Default service group** - the `default-service-group` configured on the subscriber group

Each layer only overrides fields it explicitly sets. Unset fields fall through to the next layer.

Attributes are captured as a point-in-time snapshot when the session is created. Runtime changes to a service group definition do not affect existing sessions.

## Config Fields

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `vrf` | string | VRF name (must exist in `vrfs` config) | `cgnat` |
| `unnumbered` | string | Unnumbered interface for subscriber | `loop101` |
| `urpf` | string | uRPF mode: `strict`, `loose`, or empty to disable | `strict` |
| `acl` | [ACL](#acl) | Access control list configuration | |
| `qos` | [QoS](#qos) | Quality of service configuration | |

### ACL

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `ingress` | string | Ingress ACL name | `residential-in` |
| `egress` | string | Egress ACL name | `residential-out` |

### QoS

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `ingress-policy` | string | Ingress [QoS policy](qos.md) name | `upload-50m` |
| `egress-policy` | string | Egress [QoS policy](qos.md) name | `download-200m` |
| `upload-rate` | uint64 | Upload rate limit in bps (reserved for AAA ad-hoc rates) | `1000000000` |
| `download-rate` | uint64 | Download rate limit in bps (reserved for AAA ad-hoc rates) | `1000000000` |

## AAA Attributes

Per-subscriber attributes are returned by the configured AuthProvider plugin (e.g. `subscriber.auth.local`, `subscriber.auth.http`). The attributes available depend on the AuthProvider implementation. The following attribute keys are recognised by the service group resolver:

| AAA Attribute | Overrides |
|---------------|-----------|
| `service-group` | Selects a named service group |
| `vrf` | VRF name |
| `unnumbered` | Unnumbered interface |
| `urpf` | uRPF mode |
| `acl.ingress` | Ingress ACL |
| `acl.egress` | Egress ACL |
| `qos.ingress-policy` | Ingress QoS policy |
| `qos.egress-policy` | Egress QoS policy |
| `qos.upload-rate` | Upload rate (bps) |
| `qos.download-rate` | Download rate (bps) |

## Runtime API

Service groups can be created, updated, and deleted at runtime via the northbound API. Changes only affect new sessions.

```bash
# Create or update a service group
curl -X POST http://localhost:8080/api/set/service-groups/premium \
  -d '{"vrf": "enterprise", "unnumbered": "loop200", "qos": {"download-rate": 10000000000}}'

# View all service groups
curl http://localhost:8080/api/show/service-groups
```

## Example

```yaml
qos-policies:
  residential-1g:
    cir: 1000000
    conform:
      action: transmit
    exceed:
      action: drop
    violate:
      action: drop

  enterprise-10g:
    cir: 10000000
    conform:
      action: transmit
    exceed:
      action: drop
    violate:
      action: drop

vrfs:
  cgnat:
    address-families:
      ipv4-unicast: {}

service-groups:
  cgnat-residential:
    vrf: cgnat
    unnumbered: loop101
    urpf: strict
    qos:
      ingress-policy: residential-1g
      egress-policy: residential-1g

  enterprise:
    vrf: cgnat
    unnumbered: loop101
    qos:
      ingress-policy: enterprise-10g
      egress-policy: enterprise-10g

subscriber-groups:
  groups:
    default:
      default-service-group: cgnat-residential
      vlans:
        - svlan: "100-110"
          cvlan: any
          interface: loop100
```

In this example, all subscribers in the `default` group get the `cgnat-residential` service group by default. AAA can override individual subscribers to `enterprise` by returning `service-group: enterprise`, or override specific fields like `qos.egress-policy: enterprise-10g`.
