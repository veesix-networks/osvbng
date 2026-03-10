# DHCPv6

DHCPv6 provider configuration. IANA pools, prefix delegation pools, and timing parameters are defined in [IPv6 profiles](ipv6-profiles.md), referenced by [subscriber groups](subscriber-groups.md) via `ipv6-profile`.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `provider` | string | DHCPv6 provider: `local` | `local` |
| `dns_servers` | array | Global fallback IPv6 DNS servers | `[2001:db8::53]` |
| `domain_list` | array | DNS search domain list | `[example.com]` |
| `ra` | [RA](#router-advertisement) | Router Advertisement defaults | |

## Modes

osvbng supports three DHCPv6 modes, selected per IPv6 profile:

| Mode | Description |
|------|-------------|
| `server` | Local DHCPv6 server, allocates addresses and prefixes from configured pools (default) |
| `relay` | RFC 8415 relay agent, wraps client messages in Relay-Forward envelopes with Interface-ID, Remote-ID, and Subscriber-ID options |
| `proxy` | DHCPv6 proxy, relays to external servers but manages client-facing lifetimes independently |

Mode is configured under `ipv6-profiles.<name>.dhcpv6.mode`. See [IPv6 Profiles](ipv6-profiles.md#dhcpv6-options) for the full configuration reference.

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

See [IPv6 Profiles](ipv6-profiles.md) for relay and proxy configuration examples and [DHCP Relay & Proxy](../architecture/DHCP_RELAY_PROXY.md) for architecture details.
