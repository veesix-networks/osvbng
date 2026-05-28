# Protocols <span class="version-badge">v0.2.0</span>

Routing protocol configuration.

## BGP

Most BGP fields follow [FRR BGP conventions](https://docs.frrouting.org/en/latest/bgp.html#bgp-router-configuration).

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `asn` | int | Autonomous System number for this BGP instance | `65000` |
| `router-id` | string | Override the default router ID (highest loopback address) with a fixed value in A.B.C.D format | `10.255.0.1` |
| `peer-groups` | [BGPPeerGroup](#bgp-peer-groups) | Named groups of common neighbor configuration, applied to neighbors via `peer-group` | |
| `neighbors` | [BGPNeighbor](#bgp-neighbors) | BGP neighbors keyed by IP address | |
| `ipv4-unicast` | [BGPAddressFamily](#bgp-address-family) | IPv4 unicast address family global configuration | |
| `ipv6-unicast` | [BGPAddressFamily](#bgp-address-family) | IPv6 unicast address family global configuration | |
| `vrf` | [BGPVRF](#bgp-vrf) | Per-VRF BGP instances, each with its own neighbors and address families | |

### BGP Peer Groups

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `remote-as` | int | AS number of peers in this group; if same as local ASN, creates iBGP peering | `65001` |
| `password` | string | TCP MD5 signature (RFC 2385) applied to every session in the group; per-neighbor `password` overrides this | `s3cret` |
| `ipv4-unicast` | [BGPNeighborAFI](#bgp-neighbor-afi-config) | IPv4 unicast AFI/SAFI policy for this group | |
| `ipv6-unicast` | [BGPNeighborAFI](#bgp-neighbor-afi-config) | IPv6 unicast AFI/SAFI policy for this group | |

### BGP Neighbors

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `remote-as` | int | AS number of this neighbor; determines eBGP vs iBGP behavior | `65001` |
| `peer-group` | string | Inherit configuration from a named peer group | `upstream` |
| `password` | string | TCP MD5 signature (RFC 2385) for this session; overrides any peer-group password | `s3cret` |
| `bfd` | bool | Enable Bidirectional Forwarding Detection for fast failure detection | `true` |
| `description` | string | Text description of this neighbor for operational reference | `Core Router` |
| `ipv4-unicast` | [BGPNeighborAFI](#bgp-neighbor-afi-config) | IPv4 unicast AFI/SAFI policy overrides for this neighbor | |
| `ipv6-unicast` | [BGPNeighborAFI](#bgp-neighbor-afi-config) | IPv6 unicast AFI/SAFI policy overrides for this neighbor | |

### BGP Neighbor AFI Config

Applies to neighbor and peer-group `ipv4-unicast` / `ipv6-unicast`.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `next-hop-self` | bool | Set this router as the next-hop for routes advertised to this neighbor; commonly used on iBGP peers | `true` |
| `send-community` | string | Send community attributes: `standard`, `extended`, `both`, or `all` | `both` |
| `route-policy-in` | string | Apply a route-policy to incoming route updates from this neighbor | `CUSTOMER-IN` |
| `route-policy-out` | string | Apply a route-policy to outgoing route updates to this neighbor | `CUSTOMER-OUT` |

### BGP Address Family

Applies to top-level `ipv4-unicast` / `ipv6-unicast`.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `neighbors` | [BGPNeighbor](#bgp-neighbors) | Per-neighbor AFI/SAFI policy overrides within this address family | |
| `networks` | [BGPNetwork](#bgp-network) | Networks to originate and advertise to peers (key = prefix in CIDR) | |
| `redistribute` | [BGPRedistribute](#bgp-redistribute) | Redistribute routes from other protocols into BGP | |

### BGP Network

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `route-policy` | string | Apply a route-policy when originating this network | `ADVERTISE` |

### BGP Redistribute

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `connected` | bool | Redistribute directly connected routes into BGP | `true` |
| `static` | bool | Redistribute static routes into BGP | `false` |
| `route-policy` | string | Apply a route-policy to redistributed routes | `REDIST-FILTER` |

### BGP VRF

Each key in `vrf` is a VRF name.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `router-id` | string | Router ID for this VRF BGP instance in A.B.C.D format | `10.255.0.1` |
| `rd` | string | Route distinguisher to make VPN prefixes unique across VRFs | `65000:100` |
| `ipv4-unicast` | [BGPAddressFamily](#bgp-address-family) | IPv4 unicast address family | |
| `ipv6-unicast` | [BGPAddressFamily](#bgp-address-family) | IPv6 unicast address family | |

### BGP Example

```yaml
protocols:
  bgp:
    asn: 65000
    router-id: 10.255.0.1
    peer-groups:
      upstream:
        remote-as: 65001
        ipv4-unicast:
          next-hop-self: true
          send-community: both
    neighbors:
      10.0.0.1:
        peer-group: upstream
        description: Core Router
        bfd: true
    ipv4-unicast:
      networks:
        10.100.0.0/16: {}
      redistribute:
        connected: true
```

---

## OSPF

Most OSPF fields follow [FRR OSPFv2 conventions](https://docs.frrouting.org/en/latest/ospfd.html).

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `enabled` | bool | Enable the OSPF routing process | `true` |
| `router-id` | string | Override the automatically selected router ID with a fixed value in A.B.C.D format | `10.255.0.1` |
| `areas` | [OSPFArea](#ospf-area) | OSPF areas keyed by area ID in dotted-decimal format | |
| `redistribute` | [OSPFRedistribute](#ospf-redistribute) | Redistribute routes from other protocols into OSPF | |
| `default-information` | [OSPFDefaultInfo](#ospf-default-information) | Control origination of a default route into OSPF | |
| `log-adjacency-changes` | bool | Log a message when an OSPF neighbor adjacency state changes | `true` |
| `auto-cost-reference-bandwidth` | int | Reference bandwidth in Mbps used to calculate default interface cost (cost = ref-bw / interface-bw) | `10000` |
| `maximum-paths` | int | Maximum number of equal-cost paths for ECMP load balancing | `4` |
| `default-metric` | int | Default metric applied to routes redistributed into OSPF | `10` |
| `distance` | int | Administrative distance for OSPF routes (lower values preferred over other protocols) | `110` |
| `vrf` | [OSPFVRF](#ospf-vrf) | Per-VRF OSPF instances keyed by VRF name. Each entry renders as `router ospf vrf <name>` in FRR. | |

### OSPF Area

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `interfaces` | [OSPFInterface](#ospf-interface) | Interfaces belonging to this area, keyed by interface name | |
| `authentication` | string | Area-wide authentication default. `simple` enables Type 1 (simple-password) on all area interfaces, `message-digest` enables Type 2 (MD5 HMAC). Per-interface `authentication.mode` overrides this. Key material is always per-interface. | `message-digest` |

### OSPF Interface

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `passive` | bool | Suppress sending and receiving OSPF packets on this interface; the interface's connected subnet is still advertised | `false` |
| `cost` | int | Interface cost used in SPF calculation; lower cost paths are preferred | `100` |
| `network` | string | OSPF network type: `broadcast`, `non-broadcast`, `point-to-point`, `point-to-multipoint` | `point-to-point` |
| `bfd` | bool | Enable BFD for sub-second neighbor failure detection on this interface | `true` |
| `hello-interval` | int | Interval in seconds between OSPF Hello packets sent on this interface | `10` |
| `dead-interval` | int | Time in seconds to wait without receiving Hellos before declaring a neighbor down | `40` |
| `mtu-ignore` | bool | Disable MTU mismatch detection during database exchange; useful when interface MTUs differ between neighbors | `false` |
| `priority` | int | Router priority for DR/BDR election on broadcast/NBMA networks; 0 means this router will not participate in election | `1` |
| `authentication` | [OSPFInterfaceAuth](#ospf-interface-authentication) | Per-interface authentication. Overrides any area-wide `authentication` default. | |

### OSPF Interface Authentication

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `mode` | string | `null` (explicit no-auth, overrides area default), `simple` (RFC 2328 Type 1, cleartext, max 8 chars), `message-digest` (Type 2, MD5 HMAC) | `message-digest` |
| `key` | string | Key material. Required for `simple` and `message-digest`. | `s3cret` |
| `key-id` | int | Key identifier in [1, 255] for `message-digest` mode; must match the peer | `1` |

### OSPF Redistribute

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `connected` | bool | Redistribute directly connected routes into OSPF as external LSAs | `true` |
| `static` | bool | Redistribute static routes into OSPF as external LSAs | `false` |
| `bgp` | bool | Redistribute BGP routes into OSPF as external LSAs | `false` |
| `route-policy` | string | Apply a route-policy to redistributed routes | `REDIST-FILTER` |

### OSPF Default Information

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `originate` | bool | Generate and advertise a default route (0.0.0.0/0) into OSPF | `true` |
| `always` | bool | Always advertise the default route even if one is not present in the routing table | `true` |
| `metric` | int | Metric assigned to the default route | `10` |
| `metric-type` | int | External metric type: 1 (E1, cost added to internal) or 2 (E2, cost is fixed) | `2` |

### OSPF Example

```yaml
protocols:
  ospf:
    enabled: true
    router-id: 10.255.0.1
    log-adjacency-changes: true
    auto-cost-reference-bandwidth: 10000
    areas:
      0.0.0.0:
        interfaces:
          eth2:
            network: point-to-point
            bfd: true
          loop100:
            passive: true
    redistribute:
      connected: true
    default-information:
      originate: true
      always: true
```

### OSPF VRF

A per-VRF OSPFv2 instance, scoped to a VRF declared at the top-level `vrfs:` block. Mirrors the global OSPF fields but with no `enabled` flag — presence in the `vrf:` map implies the instance exists. Interfaces referenced under a VRF entry must have their `vrf:` field on `interfaces.<*>` (or `interfaces.<*>.subinterfaces.<*>`) matching the VRF name; an interface declared in both global and a VRF block is rejected at commit.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `router-id` | string | Per-VRF router ID. RFC 2328 requires uniqueness within one routing domain; cross-instance reuse is fine. | `10.255.0.2` |
| `areas` | [OSPFArea](#ospf-area) | Areas for this VRF instance | |
| `redistribute` | [OSPFRedistribute](#ospf-redistribute) | | |
| `default-information` | [OSPFDefaultInfo](#ospf-default-information) | | |
| `log-adjacency-changes` | bool | | `true` |
| `auto-cost-reference-bandwidth` | int | | `10000` |
| `maximum-paths` | int | | `4` |
| `default-metric` | int | | `10` |
| `distance` | int | | `110` |

Example — global OSPF on WAN uplinks plus a management-VRF instance with MD5 authentication:

```yaml
protocols:
  ospf:
    enabled: true
    router-id: 10.255.0.1
    areas:
      0.0.0.0:
        interfaces:
          eth1: { network: point-to-point }
    vrf:
      MGMT-VRF:
        router-id: 10.255.0.2
        log-adjacency-changes: true
        areas:
          0.0.0.0:
            interfaces:
              eth2:
                network: point-to-point
                authentication:
                  mode: message-digest
                  key-id: 1
                  key: mgmt-secret
```

---

## OSPFv3

Most OSPFv3 fields follow [FRR OSPFv3 conventions](https://docs.frrouting.org/en/latest/ospf6d.html).

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `enabled` | bool | Enable the OSPFv3 routing process for IPv6 | `true` |
| `router-id` | string | Override the automatically selected router ID with a fixed value in A.B.C.D format | `10.255.0.1` |
| `areas` | [OSPFv3Area](#ospfv3-area) | OSPFv3 areas keyed by area ID in dotted-decimal format | |
| `redistribute` | [OSPFv3Redistribute](#ospfv3-redistribute) | Redistribute routes from other protocols into OSPFv3 | |
| `default-information` | [OSPFv3DefaultInfo](#ospfv3-default-information) | Control origination of a default route into OSPFv3 | |
| `log-adjacency-changes` | bool | Log a message when an OSPFv3 neighbor adjacency state changes | `true` |
| `auto-cost-reference-bandwidth` | int | Reference bandwidth in Mbps used to calculate default interface cost | `10000` |
| `maximum-paths` | int | Maximum number of equal-cost paths for ECMP load balancing | `4` |
| `distance` | int | Administrative distance for OSPFv3 routes | `110` |
| `vrf` | [OSPFv3VRF](#ospfv3-vrf) | Per-VRF OSPFv3 instances keyed by VRF name. Each entry renders as `router ospf6 vrf <name>` in FRR. | |

### OSPFv3 Area

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `interfaces` | [OSPFv3Interface](#ospfv3-interface) | Interfaces belonging to this area, keyed by interface name | |

### OSPFv3 Interface

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `passive` | bool | Suppress sending and receiving OSPFv3 packets on this interface; the interface's connected prefix is still advertised | `false` |
| `cost` | int | Interface cost used in SPF calculation; lower cost paths are preferred | `100` |
| `network` | string | OSPFv3 network type: `broadcast`, `point-to-multipoint`, `point-to-point` | `point-to-point` |
| `bfd` | bool | Enable BFD for sub-second neighbor failure detection on this interface | `true` |
| `hello-interval` | int | Interval in seconds between OSPFv3 Hello packets sent on this interface | `10` |
| `dead-interval` | int | Time in seconds to wait without receiving Hellos before declaring a neighbor down | `40` |
| `mtu-ignore` | bool | Disable MTU mismatch detection during database exchange | `false` |
| `priority` | int | Router priority for DR/BDR election; 0 means this router will not participate in election | `1` |
| `authentication` | [OSPFv3InterfaceAuth](#ospfv3-interface-authentication) | RFC 7166 Authentication Trailer manual key | |

### OSPFv3 Interface Authentication

OSPFv3 uses the Authentication Trailer (RFC 7166) with a manual key per interface. All three fields are required together.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `key-id` | int | Key identifier in [1, 65535]; must match the peer | `10` |
| `hash-algo` | string | `md5`, `hmac-sha-1`, `hmac-sha-256`, `hmac-sha-384`, or `hmac-sha-512`. SHA-1 and SHA-256+ require FRR built with openssl. | `hmac-sha-256` |
| `key` | string | Key material | `s3cret` |

### OSPFv3 Redistribute

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `connected` | bool | Redistribute directly connected routes into OSPFv3 | `true` |
| `static` | bool | Redistribute static routes into OSPFv3 | `false` |
| `bgp` | bool | Redistribute BGP routes into OSPFv3 | `false` |

### OSPFv3 Default Information

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `originate` | bool | Generate and advertise a default route (::/0) into OSPFv3 | `true` |
| `always` | bool | Always advertise the default route even if one is not present in the routing table | `true` |
| `metric` | int | Metric assigned to the default route | `10` |
| `metric-type` | int | External metric type: 1 (E1, cost added to internal) or 2 (E2, cost is fixed) | `2` |

### OSPFv3 Example

```yaml
protocols:
  ospf6:
    enabled: true
    router-id: 10.255.0.1
    areas:
      0.0.0.0:
        interfaces:
          eth2:
            network: point-to-point
```

### OSPFv3 VRF

A per-VRF OSPFv3 instance, scoped to a VRF declared at the top-level `vrfs:` block. Mirrors the global OSPFv3 fields but with no `enabled` flag — presence in the `vrf:` map implies the instance exists. Same interface ↔ VRF cross-validation as the OSPFv2 case.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `router-id` | string | Per-VRF router ID in A.B.C.D format | `10.255.0.2` |
| `areas` | [OSPFv3Area](#ospfv3-area) | Areas for this VRF instance | |
| `redistribute` | [OSPFv3Redistribute](#ospfv3-redistribute) | | |
| `default-information` | [OSPFv3DefaultInfo](#ospfv3-default-information) | | |
| `log-adjacency-changes` | bool | | `true` |
| `auto-cost-reference-bandwidth` | int | | `10000` |
| `maximum-paths` | int | | `4` |
| `distance` | int | | `110` |

Example:

```yaml
protocols:
  ospf6:
    enabled: true
    router-id: 10.255.0.1
    areas:
      0.0.0.0:
        interfaces:
          eth1: { network: point-to-point }
    vrf:
      MGMT-VRF:
        router-id: 10.255.0.2
        areas:
          0.0.0.0:
            interfaces:
              eth2: { network: point-to-point }
```

---

## IS-IS

Most IS-IS fields follow [FRR IS-IS conventions](https://docs.frrouting.org/en/latest/isisd.html).

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `enabled` | bool | Enable the IS-IS routing process | `true` |
| `net` | string | Network Entity Title (NET) in ISO format; encodes area ID and system ID | `49.0001.1000.0000.0001.00` |
| `is-type` | string | IS-IS router type: `level-1` (intra-area), `level-1-2` (both), `level-2-only` (inter-area/backbone) | `level-2-only` |
| `metric-style` | string | Metric style: `narrow` (original, max 63), `wide` (extended, max 16M), `transition` (send both) | `wide` |
| `log-adjacency-changes` | bool | Log a message when an IS-IS adjacency state changes | `true` |
| `dynamic-hostname` | bool | Advertise the system hostname in LSPs for easier identification in show commands | `true` |
| `set-overload-bit` | bool | Set the overload bit in LSPs to signal that this router should only be used as a transit if no alternative path exists | `false` |
| `lsp-mtu` | int | Maximum size of generated LSPs in bytes; should be less than the smallest MTU on any IS-IS interface | `1497` |
| `lsp-gen-interval` | int | Minimum interval in seconds between successive LSP regenerations | `5` |
| `lsp-refresh-interval` | int | Interval in seconds at which LSPs are periodically refreshed before they expire | `900` |
| `max-lsp-lifetime` | int | Maximum time in seconds an LSP remains valid without being refreshed | `1200` |
| `spf-interval` | int | Minimum interval in seconds between SPF calculations triggered by topology changes | `5` |
| `area-password` | string | Authentication password for Level-1 (intra-area) LSPs and SNPs | |
| `domain-password` | string | Authentication password for Level-2 (inter-area) LSPs and SNPs | |
| `redistribute` | [ISISRedistribute](#is-is-redistribute) | Redistribute routes from other protocols into IS-IS | |
| `default-information` | [ISISDefaultInfo](#is-is-default-information) | Control origination of a default route into IS-IS | |
| `interfaces` | [ISISInterface](#is-is-interface) | IS-IS interfaces keyed by interface name | |

### IS-IS Interface

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `passive` | bool | Advertise this interface's connected prefixes without sending or receiving IS-IS protocol packets | `false` |
| `metric` | int | IS-IS metric for this interface; lower metric paths are preferred in SPF calculation | `10` |
| `network` | string | Network type for this interface (e.g., `point-to-point` for directly connected routers) | `point-to-point` |
| `bfd` | bool | Enable BFD for sub-second neighbor failure detection on this interface | `true` |
| `circuit-type` | string | Adjacency type on this interface: `level-1`, `level-1-2`, `level-2` | `level-2` |
| `hello-interval` | int | Interval in seconds between IS-IS Hello (IIH) packets sent on this interface | `3` |
| `hello-multiplier` | int | Number of missed Hellos before declaring a neighbor down (hold time = hello-interval x hello-multiplier) | `3` |
| `priority` | int | Priority for Designated Intermediate System (DIS) election on broadcast networks | `64` |

### IS-IS Redistribute

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `ipv4-connected` | bool | Redistribute IPv4 connected routes into IS-IS Level-2 | `true` |
| `ipv4-static` | bool | Redistribute IPv4 static routes into IS-IS Level-2 | `false` |
| `ipv6-connected` | bool | Redistribute IPv6 connected routes into IS-IS Level-2 | `true` |
| `ipv6-static` | bool | Redistribute IPv6 static routes into IS-IS Level-2 | `false` |
| `route-policy` | string | Apply a route-policy to redistributed routes | `ISIS-REDIST-FILTER` |

### IS-IS Default Information

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `originate` | bool | Originate a default route into IS-IS | `true` |

### IS-IS Example

```yaml
protocols:
  isis:
    enabled: true
    net: 49.0001.1000.0000.0001.00
    is-type: level-2-only
    metric-style: wide
    log-adjacency-changes: true
    dynamic-hostname: true
    interfaces:
      eth2:
        network: point-to-point
        metric: 10
        bfd: true
        circuit-type: level-2
    redistribute:
      ipv4-connected: true
```

---

## Static Routes

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `ipv4` | [StaticRoute](#static-route) | List of IPv4 static routes | |
| `ipv6` | [StaticRoute](#static-route) | List of IPv6 static routes | |

### Static Route

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `destination` | string | Destination prefix in CIDR notation | `0.0.0.0/0` |
| `next-hop` | string | IP address of the next-hop router | `10.0.0.1` |
| `device` | string | Outgoing interface for directly connected destinations | `eth2` |

### Static Routes Example

```yaml
protocols:
  static:
    ipv4:
      - destination: 0.0.0.0/0
        next-hop: 10.0.0.1
      - destination: 192.168.0.0/16
        device: eth2
    ipv6:
      - destination: ::/0
        next-hop: 2001:db8::1
```
