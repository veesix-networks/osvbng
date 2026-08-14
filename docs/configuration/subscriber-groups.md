# Subscriber Groups

Defines how subscribers are grouped and configured based on VLAN. Each group binds a set of VLANs to one or more access types (IPoE, PPPoE, LAC, LNS, L2GW), address profiles, service group, and AAA policy. Both IPoE and PPPoE sessions use the same profile and service group resolution.

## Where access-types is declared

`access-types` belongs on each entry in `vlans`, not on the group. A group that declares `access-types` at the top level is rejected at config load, with one exception: an LNS-only group sets `access-types: [lns]` at the group level and must not declare `vlans`, because L2TP-terminated subscribers do not arrive by S-VLAN demux.

```yaml
subscriber-groups:
  groups:
    residential:
      vlans:
        - svlan: "100-199"
          cvlan: any
          access-types: [ipoe]      # per VLAN range
          interface: loop100
          parent-interface: eth1
    wholesale-lns:
      access-types: [lns]           # group level, LNS only, no vlans
      ipv4-profile: residential
```

## Group Settings

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `access-types` | []string | LNS-only groups. Must be exactly `[lns]`, and the group must not declare `vlans`. Every other access type declares access-types per VLAN range. | `[lns]` |
| `vlans` | [VLANRule](#vlan-rules) | VLAN matching rules | |
| `vlan-tpid` | string | Outer TPID for subscriber traffic: `dot1ad` (0x88A8, default) or `dot1q` (0x8100) | `dot1ad` |
| `ipv4-profile` | string | [IPv4 profile](ipv4-profiles.md) name | `residential` |
| `ipv6-profile` | string | [IPv6 profile](ipv6-profiles.md) name | `default-v6` |
| `session-mode` | string | Session mode: `unified` or `independent` | `unified` |
| `vrf` | string | VRF for subscribers in this group | `customers` |
| `default-service-group` | string | Default [service group](service-groups.md) for subscribers | `cgnat-residential` |
| `aaa-policy` | string | Default AAA policy name | `default-policy` |
| `ipv6` | [GroupIPv6](#group-ipv6) | IPv6 settings for this group | |
| `bgp` | [GroupBGP](#group-bgp) | BGP settings for this group | |
| `pppoe` | [GroupPPPoE](#group-pppoe) | PPPoE settings for this group | |
| `mss-clamp` | [GroupMSSClamp](#group-mss-clamp) | TCP MSS clamping for this group | |
| `dhcpv6.allow-relay-forward` | bool | Accept DHCPv6 Relay-Forward messages for this group (LDRA). Default `true`. See [DHCPv6](dhcpv6.md) | `true` |
| `l2tp.profile` | string | L2TP profile name for `lac` or `lns` ranges. See [L2TP](l2tp.md) | `wholesale` |
| `l2gw.handoff-group` | string | Default handoff group for `l2gw` ranges. See [L2GW](l2gw.md) | `isp-blue` |
| `l2gw.idle-timeout` | uint32 | Tear down a dynamic l2gw circuit after this many seconds with no traffic in either direction. `0` disables. | `3600` |

## VLAN Rules

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `svlan` | string | S-VLAN match: single ID or range | `100-199` |
| `cvlan` | string | C-VLAN match: single, range, or `any` | `any` |
| `access-types` | []string | Required. One or more of `ipoe`, `pppoe`, `lac`, `lns`, `l2gw`. The only valid multi-element combination is `[ipoe, pppoe]` (mixed access on a shared S-VLAN range); `lac`, `lns` and `l2gw` are single-element only. | `[ipoe]` |
| `interface` | string | Gateway interface for matched subscribers | `loop100` |
| `parent-interface` | string | Access interface where these subscribers arrive. May be a physical port, a bond, a VXLAN tunnel (`l2gw`), or a pseudowire headend. Required for every access type except `lns`, and must name an interface defined in `interfaces`. See [Interfaces](interfaces.md) | `eth1` |
| `vrf` | string | VRF override for this VLAN range | `customers` |
| `dhcp` | string | Named DHCP server from the top-level `dhcp.servers` list | `upstream-dhcp` |
| `trigger` | string | `l2gw` ranges only: `dhcp` (default) punts DHCPv4 and DHCPv6 on circuit miss, `packet` punts the first frame of any protocol. See [L2GW](l2gw.md) | `packet` |
| `aaa.policy` | string | AAA policy override for this VLAN range | `custom-policy` |

### One access interface

All VLAN ranges across all groups must reference the same `parent-interface`, with one exception: `l2gw` ranges are exempt, because each wholesale access operator lands on its own NNI port. A configuration mixing two access interfaces for IPoE, PPPoE or LAC is rejected at load.

## Group IPv6

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `ra` | [IPv6RA](#ipv6-ra) | Router Advertisement configuration | |

### IPv6 RA

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `managed` | bool | Set Managed (M) flag in RA | `true` |
| `other` | bool | Set Other (O) flag in RA | `true` |
| `router_lifetime` | int | Router lifetime in seconds | `1800` |
| `max_interval` | int | Max RA interval in seconds | `600` |
| `min_interval` | int | Min RA interval in seconds | `200` |
| `on_link` | bool | Advertise the access prefix on-link (L flag) for this group; overrides `dhcpv6.ra.on_link`. Default `false` (off-link, subscribers route via the BNG) | `false` |
| `unicast` | bool | Deliver periodic RAs as per-subscriber unicast vs multicast for this group; overrides `dhcpv6.ra.unicast`. Default `true` (unicast, multicast fallback when the client link-local is unknown) | `true` |

## Group BGP

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `enabled` | bool | Enable BGP for this group | `true` |
| `advertise-pools` | bool | Automatically create BGP network statements for address pools. If disabled, configure networks manually under `protocols.bgp` | `true` |
| `redistribute-connected` | bool | Redistribute connected routes into BGP | `false` |
| `network-route-policy` | string | [Route-policy](routing-policies.md) applied to BGP network statements for this group's pools | `POOL-EXPORT` |
| `redistribute-route-policy` | string | [Route-policy](routing-policies.md) applied to BGP redistribute for this group | `REDIST-FILTER` |
| `vrf` | string | VRF name for BGP advertisements | `customers` |

## Group PPPoE

PPPoE-specific settings. Only consulted for VLAN ranges whose `access-types` include `pppoe`.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `mru` | uint16 | Negotiated PPP MRU. Default `1492` (RFC 2516). Set to `1500` to negotiate baby giants on the wire via PPP-Max-Payload (RFC 4638). Range `1492` to `1500`. | `1500` |

When `mru` is greater than 1492, the BNG advertises `PPP-Max-Payload` in PADO and PADS, sets the per-session VPP interface MTU to the negotiated value, and updates the LCP local MRU to match. The BNG only advertises the tag if the client included it first in PADI, per RFC 4638 §3.

Raising `mru` above 1492 is rejected at config commit unless the parent access interface MTU is large enough to carry the resulting frame. The required parent MTU is `mru + 8` (PPPoE 6 + PPP 2) `+ 4` for outer dot1q only or `+ 8` for QinQ. Example: `mru: 1500` over dot1q requires the parent interface MTU to be at least 1512.

Every L2 device between the BNG and the subscriber CPE must also support baby giants, this is a one-time provisioning task on the access network.

## Group MSS Clamp

TCP MSS clamping for subscriber traffic. Enabled by default for every subscriber group because broken PMTUD middleboxes are common on the public internet.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `enabled` | bool | Enable MSS clamping for this group. Default `true`. | `true` |
| `subscriber-path-mtu` | uint16 | MTU of the subscriber path used to auto-derive MSS for IPoE groups. Default `1500`. PPPoE groups always use the per-session negotiated PPP MRU and ignore this field. | `1500` |
| `ipv4-mss` | uint16 | Explicit IPv4 MSS. Beats auto-derive. | `1400` |
| `ipv6-mss` | uint16 | Explicit IPv6 MSS. Beats auto-derive. | `1380` |

Auto-derived MSS values:

| Access type | Path MTU source | IPv4 MSS | IPv6 MSS |
|---|---|---|---|
| IPoE | `subscriber-path-mtu` (default 1500) | path mtu - 40 | path mtu - 60 |
| PPPoE, default `pppoe.mru` | per-session, fixed 1492 | 1452 | 1432 |
| PPPoE, `pppoe.mru: 1500` | per-session, negotiated 1500 | 1460 | 1440 |

`subscriber-path-mtu` is intentionally separate from the BNG access interface MTU. An operator running jumbo frames on the access link (e.g. for MPLS or SR-MPLS in the access path) does not need to lower it just because the subscriber CPE on the other side of that link still terminates at 1500. For non-standard subscriber paths, set `subscriber-path-mtu` explicitly per group.

Set `enabled: false` to opt out of clamping for a group, for example when every link in the subscriber path supports PMTUD properly. Operators should be aware that clamping the SYN MSS option means subscriber TCP flows will not perform PMTUD, which is the desired behaviour for typical FTTH but not for every deployment.

## Example

```yaml
ipv4-profiles:
  residential:
    gateway: 10.255.0.1
    dns:
      - 8.8.8.8
      - 8.8.4.4
    pools:
      - name: subscriber-pool
        network: 10.255.0.0/16
    dhcp:
      lease-time: 3600

ipv6-profiles:
  default-v6:
    iana-pools:
      - name: wan-link-pool
        network: 2001:db8:0:1::/64
        range_start: 2001:db8:0:1::1000
        range_end: 2001:db8:0:1::ffff
        gateway: 2001:db8:0:1::1
        preferred_time: 3600
        valid_time: 7200
    pd-pools:
      - name: subscriber-pd-pool
        network: 2001:db8:100::/40
        prefix_length: 56
        preferred_time: 3600
        valid_time: 7200
    dns:
      - 2001:4860:4860::8888
      - 2001:4860:4860::8844

service-groups:
  cgnat-residential:
    vrf: cgnat
    unnumbered: loop100
    urpf: strict

subscriber-groups:
  groups:
    residential:
      session-mode: unified
      ipv4-profile: residential
      ipv6-profile: default-v6
      default-service-group: cgnat-residential
      aaa-policy: default-policy
      vlans:
        - svlan: "100-199"
          cvlan: any
          access-types: [ipoe]
          interface: loop100
          parent-interface: eth1
      bgp:
        enabled: true
        advertise-pools: true
        network-route-policy: POOL-EXPORT
    residential-pppoe:
      ipv4-profile: residential
      ipv6-profile: default-v6
      aaa-policy: default-policy
      vlans:
        - svlan: "200-299"
          cvlan: any
          access-types: [pppoe]
          interface: loop100
          parent-interface: eth1
      pppoe:
        mru: 1500

interfaces:
  eth1:
    enabled: true
    mtu: 1512

aaa:
  auth_provider: local
  policy:
    - name: default-policy
      format: $remote-id$
```
