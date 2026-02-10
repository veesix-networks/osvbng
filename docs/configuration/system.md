# System

System-level configuration.

## CPPM (Control Plane Protection Manager)

Rate-limits control plane traffic to protect the CPU from excessive packet rates.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `dataplane` | [PlaneConfig](#plane-config) | Policers for dataplane-originated control packets | |
| `controlplane` | [PlaneConfig](#plane-config) | Policers for control plane packets | |

### Plane Config

Each plane contains a `policer` map keyed by protocol or traffic type.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `policer` | [Policer](#policer) | Policer name to rate-limit configuration | |

### Supported Policer Names

| Name | Description | Default Rate | Default Burst |
|------|-------------|-------------|---------------|
| `dhcpv4` | DHCPv4 packets | 1000 | 100 |
| `dhcpv6` | DHCPv6 packets | 1000 | 100 |
| `arp` | ARP packets | 500 | 50 |
| `pppoe` | PPPoE packets | 2000 | 200 |
| `ipv6-nd` | IPv6 Neighbor Discovery packets | 500 | 50 |
| `l2tp` | L2TP packets | 500 | 50 |

### Policer

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `rate` | float | Maximum packet rate in packets per second | `1000` |
| `burst` | int | Maximum burst size in packets | `100` |

## Example

```yaml
system:
  cppm:
    dataplane:
      policer:
        arp:
          rate: 1000
          burst: 100
        dhcpv4:
          rate: 1000
          burst: 100
        dhcpv6:
          rate: 1000
          burst: 100
        pppoe:
          rate: 2000
          burst: 200
        ipv6-nd:
          rate: 500
          burst: 50
        l2tp:
          rate: 500
          burst: 50
```
