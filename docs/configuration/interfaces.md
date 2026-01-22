# Interfaces

Network interface configuration. Each key in the `interfaces` map is an interface name.

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Interface name |
| `description` | string | Human-readable description |
| `enabled` | bool | Enable the interface |
| `mtu` | int | MTU size |
| `lcp` | bool | Create Linux Control Plane interface |
| `bng_mode` | string | BNG role: `access` or `core` |
| `address.ipv4` | array | IPv4 addresses (CIDR notation) |
| `address.ipv6` | array | IPv6 addresses (CIDR notation) |

## BNG Modes

| Mode | Description |
|------|-------------|
| `access` | Subscriber-facing interface (DHCP, PPPoE) |
| `core` | Upstream/network-facing interface (BGP, routing) |


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

  loop100:
    name: loop100
    description: Subscriber Gateway
    enabled: true
    lcp: true
    address:
      ipv4:
        - 10.255.0.1/32
```
