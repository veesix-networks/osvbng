# DHCPv6

DHCPv6 provider configuration for IPv6 address assignment and prefix delegation.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `provider` | string | DHCPv6 provider: `local` | `local` |
| `iana_pools` | [IANAPool](#iana-pools) | IANA (Identity Association for Non-temporary Addresses) address pools | |
| `pd_pools` | [PDPool](#prefix-delegation-pools) | Prefix delegation pools | |
| `dns_servers` | array | IPv6 DNS server addresses | `[2001:db8::53]` |
| `domain_list` | array | DNS search domain list | `[example.com]` |
| `ra` | [RA](#router-advertisement) | Router Advertisement defaults for DHCPv6 sessions | |

## IANA Pools

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `name` | string | Pool name, referenced by subscriber groups via `iana-pool` | `residential-v6` |
| `network` | string | IPv6 network prefix in CIDR notation | `2001:db8:100::/48` |
| `range_start` | string | First IPv6 address in the assignable range | `2001:db8:100::1` |
| `range_end` | string | Last IPv6 address in the assignable range | `2001:db8:100::ffff` |
| `gateway` | string | Default gateway address | `2001:db8:100::1` |
| `preferred_time` | int | Preferred lifetime in seconds; address remains preferred for this duration | `3600` |
| `valid_time` | int | Valid lifetime in seconds; address remains usable for this duration (must be >= preferred) | `7200` |

## Prefix Delegation Pools

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `name` | string | Pool name, referenced by subscriber groups via `pd-pool` | `residential-pd` |
| `network` | string | IPv6 network to allocate delegated prefixes from | `2001:db8:200::/40` |
| `prefix_length` | int | Length of each delegated prefix (e.g., 56 means each subscriber gets a /56) | `56` |
| `preferred_time` | int | Preferred lifetime in seconds for the delegated prefix | `3600` |
| `valid_time` | int | Valid lifetime in seconds for the delegated prefix | `7200` |

## Router Advertisement

Default RA settings applied to DHCPv6 sessions.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `managed` | bool | Set Managed (M) flag; indicates addresses are available via DHCPv6 | `true` |
| `other` | bool | Set Other (O) flag; indicates other config (DNS, etc.) is available via DHCPv6 | `true` |
| `router_lifetime` | int | Router lifetime in seconds advertised in RA; 0 means this router is not a default router | `1800` |
| `max_interval` | int | Maximum interval in seconds between unsolicited RA messages | `600` |
| `min_interval` | int | Minimum interval in seconds between unsolicited RA messages | `200` |

## Example

```yaml
dhcpv6:
  provider: local
  dns_servers:
    - 2001:db8::53
    - 2001:db8::54
  domain_list:
    - example.com
  iana_pools:
    - name: residential-v6
      network: 2001:db8:100::/48
      range_start: 2001:db8:100::1
      range_end: 2001:db8:100::ffff
      preferred_time: 3600
      valid_time: 7200
  pd_pools:
    - name: residential-pd
      network: 2001:db8:200::/40
      prefix_length: 56
      preferred_time: 3600
      valid_time: 7200
  ra:
    managed: true
    other: true
    router_lifetime: 1800
```
