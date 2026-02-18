# Subscriber Provisioning

osvbng uses a provisioning-first architecture: AAA is the source of truth for every subscriber session. DHCP and IPCP are protocol adapters that deliver addresses determined by the provisioning pipeline.

This page describes how subscriber attributes are resolved and the priority of each layer.

## Pipeline Overview

When a subscriber connects, the following steps happen in order:

1. **VLAN match** - the subscriber's S-VLAN matches a [subscriber group](subscriber-groups.md), which determines the IPv4 profile, IPv6 profile, and default service group
2. **AAA authentication** - the auth plugin returns per-subscriber attributes (IP addresses, DNS, pool overrides, service group, VRF, QoS, etc.)
3. **Service group resolution** - a three-layer merge produces the subscriber's effective service attributes (VRF, unnumbered, uRPF, ACLs, QoS)
4. **IP resolution** - `ResolveV4` / `ResolveV6` merges AAA attributes with the profile defaults to produce the final address and delivery parameters
5. **Protocol delivery** - the DHCP/IPCP provider receives the resolved result and synthesises the wire-format response

The provider never allocates addresses or consults AAA attributes directly. It only sees the resolved result.

## Service Group Resolution

See [service groups](service-groups.md) for config fields and examples.

Three layers are merged (highest priority first):

| Priority | Source | Example |
|----------|--------|---------|
| 1 (highest) | Per-subscriber AAA attributes | AAA returns `vrf: enterprise` |
| 2 | AAA-assigned service group | AAA returns `service-group: premium` |
| 3 (lowest) | Default service group | `subscriber-groups.groups.<name>.default-service-group` |

Each layer only overrides fields it explicitly sets. Unset fields fall through to the next layer.

## IPv4 Address Resolution

The resolved DHCPv4 parameters are built by merging AAA attributes with the [IPv4 profile](ipv4-profiles.md):

| Parameter | Priority 1 (AAA) | Priority 2 (Profile) |
|-----------|-------------------|----------------------|
| IP address | `ipv4_address` | Allocated from pool (see [pool selection](#pool-selection)) |
| Netmask | `ipv4_netmask` | Derived from pool network prefix (`address-model: connected-subnet`) or `/32` (`unnumbered-ptp`) |
| Gateway | `ipv4_gateway` | Pool `gateway`, then profile `gateway` |
| DNS | `dns_primary` / `dns_secondary` | Pool `dns`, then profile `dns` |
| Lease time | - | Pool `lease-time`, then profile `lease-time` (default: 3600) |

### Pool Selection

When AAA does not provide an `ipv4_address`, the system allocates from the profile's pools:

1. If AAA returns a `pool` attribute, that named pool is tried first
2. Otherwise, all pools in the profile are tried in `priority` order (lower = first)
3. The gateway IP and any `exclude` ranges are never allocated

Pool allocation is handled by a shared registry initialised at startup. The same registry is used by both IPoE (DHCP) and PPPoE (IPCP), so an IP allocated by one protocol is never double-allocated by the other.

### Collision Prevention

When AAA provides an address or prefix that falls within a pool's range, the resolve layer reserves it in the shared allocator. This prevents the pool from handing the same address to a different subscriber later. The same mechanism applies to IPv4 addresses, IPv6 IANA addresses, and IPv6 delegated prefixes. The provider's lease table acts as a second safety net - any collision is rejected at protocol response time regardless of the source.

## IPv6 Address Resolution

The resolved DHCPv6 parameters are built by merging AAA attributes with the [IPv6 profile](ipv6-profiles.md):

| Parameter | Priority 1 (AAA) | Priority 2 (Profile) |
|-----------|-------------------|----------------------|
| IANA address | `ipv6_address` | Allocated from IANA pool |
| IANA pool | `iana_pool` | First available IANA pool in profile |
| PD prefix | `ipv6_prefix` | Allocated from PD pool |
| PD pool | `pd_pool` | First available PD pool in profile |
| DNS | `ipv6_dns_primary` / `ipv6_dns_secondary` | Profile `dns`, then global `dhcpv6.dns_servers` |
| Preferred / Valid time | - | Pool timing, then profile timing (defaults: 3600 / 7200) |

## AAA Attributes

The complete list of AAA attributes recognised by osvbng. Auth plugins map their response fields to these internal attribute names.

### IP Provisioning

| Attribute | Type | Description |
|-----------|------|-------------|
| `ipv4_address` | string | Subscriber IPv4 address (bypasses pool allocation) |
| `ipv4_netmask` | string | IPv4 netmask (e.g. `255.255.0.0`) |
| `ipv4_gateway` | string | IPv4 default gateway |
| `dns_primary` | string | Primary DNS server |
| `dns_secondary` | string | Secondary DNS server |
| `ipv6_address` | string | Subscriber IPv6 IANA address |
| `ipv6_prefix` | string | Delegated IPv6 prefix (e.g. `2001:db8:100::/56`) |
| `ipv6_dns_primary` | string | Primary IPv6 DNS server |
| `ipv6_dns_secondary` | string | Secondary IPv6 DNS server |

### Pool Overrides

| Attribute | Type | Description |
|-----------|------|-------------|
| `pool` | string | Override DHCPv4 pool name (tried before profile pool order) |
| `iana_pool` | string | Override DHCPv6 IANA pool name |
| `pd_pool` | string | Override DHCPv6 PD pool name |

### Service Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `service-group` | string | Activate a named [service group](service-groups.md) |
| `vrf` | string | Override VRF |
| `unnumbered` | string | Override unnumbered interface |
| `urpf` | string | Override uRPF mode (`strict`, `loose`, or empty) |
| `acl.ingress` | string | Override ingress ACL |
| `acl.egress` | string | Override egress ACL |
| `qos.ingress-policy` | string | Override ingress QoS policy |
| `qos.egress-policy` | string | Override egress QoS policy |
| `qos.upload-rate` | uint64 | Override upload rate (bps) |
| `qos.download-rate` | uint64 | Override download rate (bps) |

### Session Control

| Attribute | Type | Description |
|-----------|------|-------------|
| `session_timeout` | int | Session timeout in seconds |
| `idle_timeout` | int | Idle timeout in seconds |

## HTTP Auth Plugin Default Mappings

The [HTTP auth plugin](plugins/auth-http.md) auto-discovers attributes from the JSON response using these default field names. You can override these with explicit `attribute_mappings`.

| Internal Attribute | Default JSON Paths |
|--------------------|--------------------|
| `ipv4_address` | `ipv4_address`, `ip_address`, `ip`, `framed_ip_address`, `subscriber.ipv4.address` |
| `ipv4_netmask` | `ipv4_netmask`, `netmask`, `framed_ip_netmask`, `subscriber.ipv4.netmask` |
| `ipv4_gateway` | `ipv4_gateway`, `gateway`, `default_gateway`, `subscriber.ipv4.gateway` |
| `dns_primary` | `dns_primary`, `dns[0]`, `dns_servers[0]`, `subscriber.dns.primary` |
| `dns_secondary` | `dns_secondary`, `dns[1]`, `dns_servers[1]`, `subscriber.dns.secondary` |
| `ipv6_address` | `ipv6_address`, `framed_ipv6_address`, `subscriber.ipv6.address` |
| `ipv6_prefix` | `ipv6_prefix`, `delegated_prefix`, `framed_ipv6_prefix`, `subscriber.ipv6.prefix` |
| `service-group` | `service_group`, `service-group`, `subscriber.service_group` |
| `vrf` | `vrf`, `routing_instance`, `subscriber.vrf` |
| `pool` | `pool`, `address_pool`, `framed_pool`, `subscriber.pool` |
| `iana_pool` | `iana_pool`, `ipv6_address_pool`, `subscriber.iana_pool` |
| `pd_pool` | `pd_pool`, `prefix_pool`, `delegated_prefix_pool`, `subscriber.pd_pool` |

## Scenarios

### Scenario 1: Default - Pool Allocation

No AAA attributes returned for IP. Subscriber gets an IP from the profile's pool.

```
Subscriber connects on SVLAN 100
  > Matches subscriber group "residential"
  > IPv4 profile: "residential" (gateway: 10.255.0.1, pool: 10.255.0.0/16)
  > AAA approves, returns no IP attributes
  > ResolveV4: allocates 10.255.0.2 from subscriber-pool
  > DHCP OFFER: IP=10.255.0.2, mask=/16, router=10.255.0.1, DNS=8.8.8.8
```

### Scenario 2: AAA-Provisioned IP

AAA returns a specific IP address. Pool is not consulted.

```
AAA response: { "ip_address": "10.255.100.50", "gateway": "10.255.0.1" }
  > ResolveV4: uses 10.255.100.50 directly (no pool allocation)
  > Gateway from AAA: 10.255.0.1
  > DNS falls through to profile: 8.8.8.8, 8.8.4.4
  > DHCP OFFER: IP=10.255.100.50, mask=/16, router=10.255.0.1, DNS=8.8.8.8
```

### Scenario 3: AAA Pool Override

AAA returns a pool name instead of a specific IP.

```
AAA response: { "pool": "overflow-pool" }
  > ResolveV4: tries "overflow-pool" first (instead of profile pool order)
  > Allocates 10.254.0.11 from overflow-pool
  > Gateway from overflow-pool config: 10.254.0.1
```

### Scenario 4: Service Group Activation

AAA assigns a service group that changes the VRF.

```
Config:
  service-groups:
    customer-a:
      vrf: CUSTOMER-A
      unnumbered: loop101

AAA response: { "service_group": "customer-a" }
  > Service group resolver: activates "customer-a"
  > Subscriber session placed in VRF "CUSTOMER-A" with unnumbered loop101
  > IPv4 profile still determines IP/DNS/lease-time
```

### Scenario 5: Per-Subscriber VRF Override

AAA overrides VRF directly without a service group.

```
Config:
  service-groups:
    cgnat-residential:
      vrf: cgnat
      unnumbered: loop100

Subscriber group default-service-group: cgnat-residential

AAA response: { "vrf": "CUSTOMER-A" }
  > Service group resolver:
    1. Starts with cgnat-residential (vrf=cgnat, unnumbered=loop100)
    2. AAA attribute vrf=CUSTOMER-A overrides VRF only
  > Result: vrf=CUSTOMER-A, unnumbered=loop100 (inherited from default)
```

### Scenario 6: Full AAA Override

AAA returns IP, DNS, VRF, and QoS, all overriding profile/service-group defaults.

```
AAA response: {
  "ip_address": "192.168.1.100",
  "gateway": "192.168.1.1",
  "netmask": "255.255.255.0",
  "dns_primary": "10.0.0.53",
  "service_group": "enterprise",
  "qos.download-rate": 10000000000
}
  > IP: 192.168.1.100 (AAA-provided, no pool allocation)
  > Gateway: 192.168.1.1 (AAA override)
  > Netmask: /24 (AAA override)
  > DNS: 10.0.0.53 (AAA override, only primary; secondary empty)
  > Service group: "enterprise" applied first, then qos.download-rate overrides
```

### Scenario 7: Dual-Stack with AAA

AAA provides IPv4 address and IPv6 pool overrides.

```
AAA response: {
  "ip_address": "10.255.50.1",
  "iana_pool": "business-v6",
  "pd_pool": "business-pd"
}
  > DHCPv4: uses 10.255.50.1 directly
  > DHCPv6 IANA: allocates from "business-v6" pool instead of profile default
  > DHCPv6 PD: allocates from "business-pd" pool instead of profile default
```
