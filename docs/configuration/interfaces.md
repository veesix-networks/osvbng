# Interfaces

Network interface configuration. Each key in the `interfaces` map is an interface name.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `name` | string | Interface name | `eth1` |
| `description` | string | Human-readable description | `Access Interface` |
| `enabled` | bool | Enable the interface | `true` |
| `mtu` | int | MTU size | `9000` |
| `lcp` | bool | Create Linux Control Plane interface | `true` |
| `bng_mode` | string | BNG role: `access` or `core` | `access` |
| `unnumbered` | string | Borrow address from named interface | `loop100` |
| `address` | [Address](#address) | IP address configuration | |
| `subinterfaces` | [Subinterface](#sub-interfaces) | Sub-interface configuration | |
| `ipv6` | [IPv6](#ipv6) | IPv6 configuration | |
| `arp` | [ARP](#arp) | ARP configuration | |

## Address

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `ipv4` | array | IPv4 addresses (CIDR notation) | `[10.255.0.1/32]` |
| `ipv6` | array | IPv6 addresses (CIDR notation) | `[2001:db8::1/128]` |

## Sub-interfaces

Each key in `subinterfaces` is a sub-interface name.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `vlan` | int | VLAN ID | `100` |
| `enabled` | bool | Enable the sub-interface | `true` |
| `address` | [Address](#address) | IP address configuration | |
| `ipv6` | [IPv6](#ipv6) | IPv6 configuration | |
| `arp` | [ARP](#arp) | ARP configuration | |
| `unnumbered` | string | Borrow address from named interface | `loop100` |
| `bng` | [BNG](#sub-interface-bng) | BNG configuration | |

!!! info "Automatic sub-interface management"
    When using the BNG functionality of osvbng with subscriber groups, sub-interfaces are automatically deployed and managed based on the VLAN matching rules. You do not need to manually configure sub-interfaces in this section.

### Sub-interface BNG

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `mode` | string | BNG mode: `ipoe`, `ipoe-l3`, `pppoe`, `lac`, `lns` | `pppoe` |

## IPv6

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `enabled` | bool | Enable IPv6 | `true` |
| `multicast` | bool | Enable IPv6 multicast | `true` |
| `ra` | [RA](#router-advertisement) | Router Advertisement configuration | |

### Router Advertisement

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `managed` | bool | Set Managed (M) flag in RA | `true` |
| `other` | bool | Set Other (O) flag in RA | `true` |
| `router-lifetime` | int | Router lifetime in seconds | `1800` |
| `max-interval` | int | Max RA interval in seconds | `600` |
| `min-interval` | int | Min RA interval in seconds | `200` |

## ARP

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `enabled` | bool | Enable ARP | `true` |

## Example

```yaml
interfaces:
  eth1:
    name: eth1
    description: Access Interface
    enabled: true
    bng_mode: access

  eth2:
    name: eth2
    description: Core Interface
    enabled: true
    bng_mode: core
    lcp: true
    subinterfaces:
      eth2.100:
        vlan: 100
        enabled: true
        bng:
          mode: pppoe

  loop100:
    name: loop100
    description: Subscriber Gateway
    enabled: true
    lcp: true
    address:
      ipv4:
        - 10.255.0.1/32
    ipv6:
      enabled: true
      ra:
        managed: true
        other: true
        router-lifetime: 1800
```
