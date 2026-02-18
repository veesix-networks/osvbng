# Subscriber Groups

Defines how subscribers are grouped and configured based on VLAN. Each group binds a set of VLANs to an access type (IPoE or PPPoE), address profiles, service group, and AAA policy. Both IPoE and PPPoE sessions use the same profile and service group resolution.

## Group Settings

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `access-type` | string | Access type: `ipoe`, `pppoe`, `lac`, `lns` | `ipoe` |
| `vlans` | [VLANRule](#vlan-rules) | VLAN matching rules | |
| `ipv4-profile` | string | [IPv4 profile](ipv4-profiles.md) name | `residential` |
| `ipv6-profile` | string | [IPv6 profile](ipv6-profiles.md) name | `default-v6` |
| `session-mode` | string | Session mode: `unified` or `independent` | `unified` |
| `default-service-group` | string | Default [service group](service-groups.md) for subscribers | `cgnat-residential` |
| `aaa-policy` | string | Default AAA policy name | `default-policy` |
| `ipv6` | [GroupIPv6](#group-ipv6) | IPv6 settings for this group | |
| `bgp` | [GroupBGP](#group-bgp) | BGP settings for this group | |

## VLAN Rules

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `svlan` | string | S-VLAN match: single, range, or `any` | `100-199` |
| `cvlan` | string | C-VLAN match: single, range, or `any` | `any` |
| `interface` | string | Gateway interface for matched subscribers | `loop100` |
| `aaa.policy` | string | AAA policy override for this VLAN range | `custom-policy` |

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

## Group BGP

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `enabled` | bool | Enable BGP for this group | `true` |
| `advertise-pools` | bool | Advertise address pools via BGP | `true` |
| `redistribute-connected` | bool | Redistribute connected routes | `false` |
| `vrf` | string | VRF name for BGP advertisements | `customers` |

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
      access-type: ipoe
      session-mode: unified
      ipv4-profile: residential
      ipv6-profile: default-v6
      default-service-group: cgnat-residential
      aaa-policy: default-policy
      vlans:
        - svlan: "100-199"
          cvlan: any
          interface: loop100
      bgp:
        enabled: true
        advertise-pools: true
```
