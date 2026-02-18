# IPv6 Profiles

IPv6 address profiles define IANA pools, prefix delegation pools, and DNS settings used for subscriber IPv6 provisioning. Profiles are protocol-agnostic; the same profile is used whether the subscriber connects via IPoE (DHCPv6) or PPPoE (IPv6CP). Protocol-specific options (e.g. DHCPv6 timers) are nested under a `dhcpv6` sub-key.

Profiles are referenced by [subscriber groups](subscriber-groups.md) via `ipv6-profile`.

## Profile Settings

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `iana-pools` | [IANAPool](#iana-pools) | IPv6 address pools | |
| `pd-pools` | [PDPool](#prefix-delegation-pools) | Prefix delegation pools | |
| `dns` | array | Profile-level IPv6 DNS servers | `[2001:4860:4860::8888]` |
| `ra` | [RA](dhcpv6.md#router-advertisement) | Profile-level RA overrides | |
| `dhcpv6` | [DHCPv6Options](#dhcpv6-options) | DHCPv6-specific delivery options | |
| `ipv6cp` | [IPv6CPOptions](#ipv6cp-options) | IPv6CP-specific delivery options (reserved) | |

## DHCPv6 Options

Protocol-specific settings applied only when delivering addresses via DHCPv6.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `mode` | string | Profile mode (default: `server`) | `server` |
| `preferred-time` | int | Default preferred lifetime in seconds (default: 3600) | `3600` |
| `valid-time` | int | Default valid lifetime in seconds (default: 7200) | `7200` |

## IPv6CP Options

Reserved for future IPv6CP-specific delivery options when provisioning PPPoE subscribers.

## IANA Pools

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `name` | string | Pool name | `residential-v6` |
| `network` | string | IPv6 network prefix in CIDR notation | `2001:db8:100::/48` |
| `range_start` | string | First IPv6 address in the assignable range | `2001:db8:100::1` |
| `range_end` | string | Last IPv6 address in the assignable range | `2001:db8:100::ffff` |
| `gateway` | string | Default gateway address | `2001:db8:100::1` |
| `preferred_time` | int | Preferred lifetime in seconds | `3600` |
| `valid_time` | int | Valid lifetime in seconds (must be >= preferred) | `7200` |

## Prefix Delegation Pools

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `name` | string | Pool name | `residential-pd` |
| `network` | string | IPv6 network to allocate delegated prefixes from | `2001:db8:200::/40` |
| `prefix_length` | int | Length of each delegated prefix | `56` |
| `preferred_time` | int | Preferred lifetime in seconds | `3600` |
| `valid_time` | int | Valid lifetime in seconds | `7200` |

## Example

```yaml
ipv6-profiles:
  default-v6:
    iana-pools:
      - name: wan-link-pool
        network: 2001:db8:0:1::/64
        range_start: 2001:db8:0:1::1000
        range_end: 2001:db8:0:1::ffff
        gateway: 2001:db8:0:1::1
        preferred_time: 3600
        valid_time: 7200
    pd-pools:
      - name: subscriber-pd-pool
        network: 2001:db8:100::/40
        prefix_length: 56
        preferred_time: 3600
        valid_time: 7200
    dns:
      - 2001:4860:4860::8888
      - 2001:4860:4860::8844
    dhcpv6:
      preferred-time: 3600
      valid-time: 7200
```
