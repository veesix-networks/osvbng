# DHCP

DHCP provider configuration.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `provider` | string | DHCP provider: `local` | `local` |
| `pools` | [DHCPPool](#dhcp-pools) | Local DHCP pools | |

## DHCP Pools

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `name` | string | Pool name | `residential` |
| `network` | string | Network CIDR | `10.100.0.0/16` |
| `range_start` | string | First IP in range | `10.100.1.1` |
| `range_end` | string | Last IP in range | `10.100.255.254` |
| `gateway` | string | Default gateway | `10.100.0.1` |
| `dns_servers` | array | DNS server IPs | `[192.168.100.10, 192.168.101.10]` |
| `lease_time` | int | Lease time in seconds | `3600` |

## Example

```yaml
dhcp:
  provider: local
  pools:
    - name: residential
      network: 10.100.0.0/16
      range_start: 10.100.1.1
      range_end: 10.100.255.254
      gateway: 10.100.0.1
      dns_servers:
        - 192.168.100.10
        - 192.168.101.10
      lease_time: 3600
```

!!! tip "Using subscriber-groups instead"
    Most deployments should use `subscriber-groups` with `auto-generate: true` instead of manually defining DHCP pools here. The address pools defined in subscriber groups will automatically create DHCP pools.
