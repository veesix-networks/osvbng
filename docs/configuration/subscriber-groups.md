# Subscriber Groups

Defines how subscribers are grouped and configured based on VLAN.

## Group Settings

| Field | Type | Description |
|-------|------|-------------|
| `vlans` | array | VLAN matching rules |
| `address-pools` | array | IP address pools (used for DHCP and BGP auto config) |
| `dhcp` | object | DHCP settings for this group |
| `bgp` | object | BGP settings for this group |
| `aaa-policy` | string | Default AAA policy name |

## VLAN Rules

| Field | Type | Description |
|-------|------|-------------|
| `svlan` | string | S-VLAN match: single (`100`), range (`100-200`), or `any` |
| `cvlan` | string | C-VLAN match: single (`10`), range (`10-20`), or `any` |
| `interface` | string | Gateway interface for matched subscribers |
| `dhcp` | string | DHCP server name override |
| `aaa.enabled` | bool | Enable AAA for this VLAN |
| `aaa.policy` | string | AAA policy override |

## Address Pools

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Pool name |
| `network` | string | Network CIDR (e.g., `10.255.0.0/16`) |
| `gateway` | string | Gateway IP address |
| `dns` | array | DNS server IPs |
| `priority` | int | Pool priority (lower = preferred) |

## Group DHCP

| Field | Type | Description |
|-------|------|-------------|
| `auto-generate` | bool | Auto-generate DHCP pools/config |
| `lease-time` | string | Lease time in seconds |

## Group BGP

| Field | Type | Description |
|-------|------|-------------|
| `enabled` | bool | Enable BGP for this group |
| `advertise-pools` | bool | Advertise address pools via BGP |
| `redistribute-connected` | bool | Redistribute connected routes |

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
      vlans:
        - svlan: "100-199"
          cvlan: any
          interface: loop100
      address-pools:
        - name: residential-pool
          network: 10.100.0.0/16
          gateway: 10.100.0.1
          dns:
            - 8.8.8.8
            - 8.8.4.4
          priority: 1
      dhcp:
        auto-generate: true
        lease-time: "3600"
      bgp:
        enabled: true
        advertise-pools: true
      aaa-policy: default-policy
```
