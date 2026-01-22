# Protocols

Routing protocol configuration.

## BGP

| Field | Type | Description |
|-------|------|-------------|
| `asn` | int | Local AS number |
| `router-id` | string | BGP router ID |
| `peer-groups` | map | Named peer groups |
| `neighbors` | map | BGP neighbors (key = neighbor IP) |
| `ipv4-unicast` | object | IPv4 unicast address family |
| `ipv6-unicast` | object | IPv6 unicast address family |

### BGP Neighbors

| Field | Type | Description |
|-------|------|-------------|
| `remote-as` | int | Neighbor AS number |
| `peer-group` | string | Peer group name |
| `bfd` | bool | Enable BFD |
| `description` | string | Neighbor description |

### BGP Address Family

| Field | Type | Description |
|-------|------|-------------|
| `next-hop-self` | bool | Set next-hop to self |
| `send-community` | string | Send community: `standard`, `extended`, `both`, `all` |

## Example

```yaml
protocols:
  bgp:
    asn: 65000
    router-id: 10.255.0.1
    neighbors:
      10.0.0.1:
        remote-as: 65001
        description: Core Router
        bfd: true
```
