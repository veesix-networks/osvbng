# DHCP

DHCP provider configuration.

| Field | Type | Description |
|-------|------|-------------|
| `provider` | string | DHCP provider: `local` |
| `default_server` | string | Default DHCP server name |
| `pools` | array | Local DHCP pools |


## DHCP Pools

For local mode, configure address pools.

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Pool name |
| `network` | string | Network CIDR |
| `range_start` | string | First IP in range |
| `range_end` | string | Last IP in range |
| `gateway` | string | Default gateway |
| `dns_servers` | array | DNS server IPs |
| `lease_time` | int | Lease time in seconds |

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
        - 8.8.8.8
        - 8.8.4.4
      lease_time: 3600
```

!!! tip "Using subscriber-groups instead"
    Most deployments should use `subscriber-groups` with `auto-generate: true` instead of manually defining DHCP pools here. The address pools defined in subscriber groups will automatically create DHCP pools.
