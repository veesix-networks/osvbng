# DHCPv6

DHCPv6 provider configuration. IANA pools, prefix delegation pools, and timing parameters are defined in [IPv6 profiles](ipv6-profiles.md), referenced by [subscriber groups](subscriber-groups.md) via `ipv6-profile`.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `provider` | string | DHCPv6 provider: `local` | `local` |
| `dns_servers` | array | Global fallback IPv6 DNS servers | `[2001:db8::53]` |
| `domain_list` | array | DNS search domain list | `[example.com]` |
| `ra` | [RA](#router-advertisement) | Router Advertisement defaults | |

## Router Advertisement

Default RA settings applied to subscriber sessions.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `managed` | bool | Set Managed (M) flag; indicates addresses are available via DHCPv6 | `true` |
| `other` | bool | Set Other (O) flag; indicates other config (DNS, etc.) is available via DHCPv6 | `true` |
| `router_lifetime` | int | Router lifetime in seconds advertised in RA; 0 means not a default router | `1800` |
| `max_interval` | int | Maximum interval in seconds between unsolicited RA messages | `600` |
| `min_interval` | int | Minimum interval in seconds between unsolicited RA messages | `200` |

## Example

```yaml
dhcpv6:
  provider: local
  dns_servers:
    - 2001:4860:4860::8888
    - 2001:4860:4860::8844
  ra:
    managed: true
    other: true
    router_lifetime: 1800
    max_interval: 600
    min_interval: 200
```
