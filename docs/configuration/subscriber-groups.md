# Subscriber Groups

Defines how subscribers are grouped and configured based on VLAN.

## Group Settings

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `access-type` | string | Access type: `ipoe`, `pppoe`, `lac`, `lns` | `ipoe` |
| `vlans` | [VLANRule](#vlan-rules) | VLAN matching rules | |
| `address-pools` | [AddressPool](#address-pools) | IP address pools (used for DHCP and BGP auto config) | |
| `iana-pool` | string | DHCPv6 IANA pool name | `residential-v6` |
| `pd-pool` | string | DHCPv6 prefix delegation pool name | `residential-pd` |
| `session-mode` | string | Session mode: `unified` or `independent` | `unified` |
| `dhcp` | [GroupDHCP](#group-dhcp) | DHCP settings for this group | |
| `ipv6` | [GroupIPv6](#group-ipv6) | IPv6 settings for this group | |
| `bgp` | [GroupBGP](#group-bgp) | BGP settings for this group | |
| `vrf` | string | VRF name for this group (legacy, prefer `default-service-group`) | `customers` |
| `default-service-group` | string | Default [service group](service-groups.md) for subscribers in this group | `cgnat-residential` |
| `aaa-policy` | string | Default AAA policy name | `default-policy` |

## VLAN Rules

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `svlan` | string | S-VLAN match: single, range, or `any` | `100-199` |
| `cvlan` | string | C-VLAN match: single, range, or `any` | `any` |
| `interface` | string | Gateway interface for matched subscribers | `loop100` |
| `aaa.policy` | string | AAA policy override | `custom-policy` |

## Address Pools

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `name` | string | Pool name | `residential-pool` |
| `network` | string | Network CIDR | `10.100.0.0/16` |
| `gateway` | string | Gateway IP address | `10.100.0.1` |
| `dns` | array | DNS server IPs | `[192.168.100.10, 192.168.101.10]` |
| `priority` | int | Pool priority (lower = preferred) | `1` |

## Group DHCP

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `auto-generate` | bool | Auto-generate DHCP pools/config | `true` |
| `lease-time` | string | Lease time in seconds | `"3600"` |

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

!!! info "Auto-generation"
    When `dhcp.auto-generate: true` and `bgp.advertise-pools: true` are set, osvbng automatically:

    - Creates DHCP pools from the `address-pools` (full network range, gateway, DNS)
    - Advertises the pool networks via BGP to your upstream routers

    This means you only define your address pools once, and both DHCP and routing are configured automatically.

## Example

```yaml
subscriber-groups:
  groups:
    residential:
      access-type: ipoe
      session-mode: unified
      vlans:
        - svlan: "100-199"
          cvlan: any
          interface: loop100
      address-pools:
        - name: residential-pool
          network: 10.100.0.0/16
          gateway: 10.100.0.1
          dns:
            - 192.168.100.10
            - 192.168.101.10
          priority: 1
      iana-pool: residential-v6
      pd-pool: residential-pd
      dhcp:
        auto-generate: true
        lease-time: "3600"
      ipv6:
        ra:
          managed: true
          other: true
          router_lifetime: 1800
      bgp:
        enabled: true
        advertise-pools: true
      default-service-group: cgnat-residential
      aaa-policy: default-policy
```
