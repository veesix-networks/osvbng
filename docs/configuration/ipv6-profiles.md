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

| Field | Type | Description | Default | Example |
|-------|------|-------------|---------|---------|
| `mode` | string | `server`, `relay`, or `proxy` | `server` | `relay` |
| `preferred-time` | int | Default preferred lifetime in seconds (server mode) | `3600` | `3600` |
| `valid-time` | int | Default valid lifetime in seconds (server mode) | `7200` | `7200` |
| `servers` | array | Upstream DHCPv6 servers (relay/proxy only) | | see below |
| `link-address` | string | IPv6 link address for Relay-Forward envelope | `::` | `2001:db8::1` |
| `server-timeout` | duration | Time to wait for upstream server response | `5s` | `5s` |
| `client-preferred-lifetime` | int | Preferred lifetime offered to clients (proxy only), in seconds | `300` | `300` |
| `client-valid-lifetime` | int | Valid lifetime offered to clients (proxy only), in seconds | `300` | `600` |
| `interface-id-format` | string | Interface-ID option format string (relay/proxy) | `{interface}:{svlan}:{cvlan}` | `{interface}:{svlan}:{cvlan}` |
| `remote-id-format` | string | Remote-ID option format string (relay/proxy) | | `{mac}` |
| `subscriber-id-format` | string | Subscriber-ID option format string (relay/proxy) | | `{mac}` |
| `dead-time` | duration | How long to quarantine a failed server before retrying | `30s` | `30s` |
| `dead-threshold` | int | Consecutive failures before marking a server dead | `3` | `3` |

### Servers

Each entry in the `servers` array:

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `address` | string | Server address in `[host]:port` format | `[2001:db8::100]:547` |
| `priority` | int | Higher priority servers are tried first | `100` |

### Relay Options

DHCPv6 relay wraps client messages in a Relay-Forward envelope (message type 12) per RFC 8415. The envelope includes:

- **Link-Address** - configured via `link-address`, used by the server to select the correct subnet
- **Interface-ID** (option 18) - identifies the access interface, formatted from `interface-id-format`
- **Remote-ID** (option 37) - optional, formatted from `remote-id-format`
- **Subscriber-ID** (option 38) - optional, formatted from `subscriber-id-format`

Format variables: `{interface}`, `{svlan}`, `{cvlan}`, `{mac}`.

The relay agent binds on UDP port 547 per RFC 8415 section 8.1.

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

## Examples

### Local Server

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

### Relay

Wraps DHCPv6 client messages in Relay-Forward envelopes and forwards to an external server. The server uses the link-address to select the correct subnet for address allocation.

```yaml
ipv6-profiles:
  relay-v6:
    dns:
      - 2001:4860:4860::8888
    dhcpv6:
      mode: relay
      servers:
        - address: "[2001:db8::100]:547"
          priority: 100
      link-address: "2001:db8::1"
      interface-id-format: "{interface}:{svlan}:{cvlan}"
      remote-id-format: "{mac}"
```

### Proxy

Relays to an external server but presents osvbng as the DHCPv6 server to the client. osvbng generates its own DUID from the SRG virtual MAC (HA enabled) or access interface MAC (standalone), rewrites the Server-ID in replies, and offers shorter lifetimes to clients while maintaining the full upstream lease.

When a client sends a Renew or Rebind, the proxy answers locally without contacting the upstream server — as long as the upstream server's T1 (half the server preferred lifetime) has not elapsed. Once T1 is reached, the next renewal is forwarded upstream to refresh the server-side lease.

For example, with `client-preferred-lifetime: 900` (15 minutes) and a 7-day upstream preferred lifetime:
- Clients renew every ~7.5 minutes (T1 of the 15-minute client lifetime)
- The proxy answers these renewals locally
- After 3.5 days (T1 of the 7-day server lifetime), the proxy forwards the next renewal upstream

```yaml
ipv6-profiles:
  proxy-v6:
    dns:
      - 2001:4860:4860::8888
    dhcpv6:
      mode: proxy
      servers:
        - address: "[2001:db8::100]:547"
          priority: 100
      link-address: "2001:db8::1"
      client-preferred-lifetime: 900
      client-valid-lifetime: 1800
      interface-id-format: "{interface}:{svlan}:{cvlan}"
```

### Server Failover

Both relay and proxy modes support server failover with the same logic as DHCPv4.

```yaml
ipv6-profiles:
  resilient-v6:
    dhcpv6:
      mode: relay
      servers:
        - address: "[2001:db8::100]:547"
          priority: 100
        - address: "[2001:db8::101]:547"
          priority: 50
      link-address: "2001:db8::1"
      server-timeout: 3s
      dead-time: 60s
      dead-threshold: 5
```
