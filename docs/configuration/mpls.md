# MPLS <span class="version-badge">v0.2.0</span>

MPLS label switching. osvbng uses [FRR](https://docs.frrouting.org/) as the control plane and VPP as the dataplane. Labels are distributed by LDP; transport labels combine with BGP VPN labels to carry [L3VPN](../examples/l3vpn.md) traffic.

Enabling MPLS creates the MPLS FIB, sets the kernel platform label limit, and enables MPLS forwarding on every interface that runs OSPF or IS-IS. There is no per-interface MPLS key: the IGP interface set is the MPLS interface set.

## `protocols.mpls`

| Field | Type | Default | Description | Example |
|-------|------|---------|-------------|---------|
| `enabled` | bool | `false` | Enable MPLS forwarding in the dataplane | `true` |
| `platform-labels` | uint32 | `1048575` | Highest usable platform label. Must be at least 16 | `1048575` |

```yaml
protocols:
  mpls:
    enabled: true
```

## `protocols.ldp`

Label Distribution Protocol. Fields follow [FRR LDP conventions](https://docs.frrouting.org/en/latest/ldpd.html). LDP requires `protocols.mpls.enabled: true`.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `enabled` | bool | Enable the LDP instance | `true` |
| `router-id` | string | LDP router ID in A.B.C.D format | `10.254.0.1` |
| `address-families` | [LDPAddressFamilies](#ldp-address-families) | Per-address-family transport and label advertisement | |
| `discovery-hello-holdtime` | uint32 | Hold time advertised in discovery Hellos, in seconds | `15` |
| `discovery-hello-interval` | uint32 | Interval between discovery Hellos, in seconds | `5` |
| `ordered-control` | bool | Use ordered label distribution control instead of independent | `true` |
| `dual-stack-prefer-ipv4` | bool | Prefer IPv4 for the transport connection when both families are configured | `true` |
| `neighbors` | [LDPNeighbor](#ldp-neighbors) | Per-neighbor overrides, keyed by neighbor LSR ID | |

### LDP Address Families

Keys are `ipv4` and `ipv6`. An address family must be present for LDP to run over it. The interfaces enrolled in each address family are taken from the non-passive OSPF and IS-IS interfaces, not listed here.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `transport-address` | string | Address advertised as the LDP transport endpoint. Normally the loopback carrying the router ID | `10.254.0.1` |
| `label-local-advertise` | string | Passed through to FRR's `label local advertise` for this address family | `explicit-null` |

### LDP Neighbors

Keyed by the neighbor's LSR ID.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `password` | string | TCP MD5 signature (RFC 2385) for the session with this neighbor | `s3cret` |
| `session-holdtime` | uint32 | Session hold time for this neighbor, in seconds | `180` |

## Example

```yaml
protocols:
  mpls:
    enabled: true
  ldp:
    enabled: true
    router-id: 10.254.0.1
    address-families:
      ipv4:
        transport-address: 10.254.0.1
  ospf:
    enabled: true
    router-id: 10.254.0.1
    areas:
      "0.0.0.0":
        interfaces:
          eth1:
            network: point-to-point
          loop0:
            passive: true
```

`eth1` runs OSPF and is therefore MPLS-enabled and enrolled in LDP. `loop0` is passive, so it is MPLS-enabled but does not form LDP adjacencies.

## Show commands

| Path | Description |
|------|-------------|
| `protocols.mpls.table` | MPLS forwarding table |
| `protocols.mpls.interfaces` | Interfaces with MPLS forwarding enabled |
| `protocols.ldp.neighbors` | LDP neighbors, with `detail` and `capabilities` sub-paths |
| `protocols.ldp.bindings` | Label bindings, filterable by prefix |
| `protocols.ldp.discovery` | Discovery Hello adjacencies |
| `protocols.ldp.capabilities` | Capabilities advertised by this LSR |
| `protocols.ldp.igp-sync` | LDP/IGP synchronisation state |
| `protocols.ldp.interface` | Per-interface LDP state |

## Not implemented

Segment Routing (SR-MPLS and SRv6) is not configurable. There is no `segment-routing` block in the config schema, and neither IS-IS nor OSPF accepts SR parameters.

## See also

- [MPLS L3VPN example](../examples/l3vpn.md)
- [Protocols](protocols.md) for BGP VPN address families and IGP configuration
- [VRFs](vrfs.md)
