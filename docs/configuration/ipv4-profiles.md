# IPv4 Profiles

IPv4 address profiles define pools, gateway, and DNS settings used for subscriber IP provisioning. Profiles are protocol-agnostic; the same profile is used whether the subscriber connects via IPoE (DHCP) or PPPoE (IPCP). Protocol-specific options (e.g. DHCP lease time) are nested under a `dhcp` sub-key.

Profiles are referenced by [subscriber groups](subscriber-groups.md) via `ipv4-profile`.

## Profile Settings

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `gateway` | string | Default gateway IP for all pools in this profile | `10.255.0.1` |
| `dns` | array | DNS server IPs (pool-level overrides profile-level) | `[8.8.8.8, 8.8.4.4]` |
| `pools` | [IPv4Pool](#ipv4-pools) | Address pools for this profile | |
| `dhcp` | [DHCPOptions](#dhcp-options) | DHCP-specific delivery options | |
| `ipcp` | [ICPPOptions](#ipcp-options) | IPCP-specific delivery options (reserved) | |

## DHCP Options

Protocol-specific settings applied only when delivering addresses via DHCPv4.

| Field | Type | Description | Default | Example |
|-------|------|-------------|---------|---------|
| `mode` | string | `server`, `relay`, or `proxy` | `server` | `relay` |
| `address-model` | string | `connected-subnet` or `unnumbered-ptp` | `connected-subnet` | `connected-subnet` |
| `server-id` | string | DHCP server identifier (server mode only); defaults to gateway | | `10.255.0.1` |
| `lease-time` | int | Lease time in seconds (server mode only) | `3600` | `3600` |
| `servers` | array | Upstream DHCP servers (relay/proxy only) | | see below |
| `giaddr` | string | Relay agent IP inserted into GIAddr field | | `10.0.0.1` |
| `server-timeout` | duration | Time to wait for upstream server response | `5s` | `5s` |
| `client-lease` | int | Lease time offered to clients (proxy only), in seconds | `300` | `300` |
| `option82` | [Option82](#option-82) | Option 82 (Relay Agent Information) settings | | see below |
| `dead-time` | duration | How long to quarantine a failed server before retrying | `30s` | `30s` |
| `dead-threshold` | int | Consecutive failures before marking a server dead | `3` | `3` |

### Servers

Each entry in the `servers` array:

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `address` | string | Server address in `host:port` format | `192.168.1.1:67` |
| `priority` | int | Higher priority servers are tried first | `100` |

### Option 82

Relay Agent Information option (RFC 3046). Supported in both relay and proxy modes.

| Field | Type | Description | Default | Example |
|-------|------|-------------|---------|---------|
| `circuit-id-format` | string | Circuit ID sub-option format string | `{interface}:{svlan}:{cvlan}` | `{interface}:{svlan}:{cvlan}` |
| `remote-id-format` | string | Remote ID sub-option format string | `{mac}` | `{mac}` |
| `policy` | string | `keep`, `replace`, or `drop` | `replace` | `keep` |
| `include-flags` | bool | Include flags sub-option (unicast flag) | `false` | `true` |

Format variables: `{interface}`, `{svlan}`, `{cvlan}`, `{mac}`.

**Policies:**

- `keep` - if the packet already contains Option 82 (e.g. inserted by a DSLAM or access node), leave it untouched
- `replace` - remove any existing Option 82 and insert osvbng's own (default)
- `drop` - remove existing Option 82 without inserting a replacement

### Address Models

- **`connected-subnet`** (default) - subscriber gets an IP on a shared subnet. Netmask is derived from the pool's network prefix.
- **`unnumbered-ptp`** - subscriber gets a `/32` address. A classless static route via the gateway is included automatically.

## IPCP Options

Reserved for future IPCP-specific delivery options when provisioning PPPoE subscribers.

## IPv4 Pools

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `name` | string | Pool name (must be unique across all profiles) | `residential` |
| `network` | string | Network CIDR | `10.100.0.0/16` |
| `range-start` | string | First allocatable IP (default: first host in network) | `10.100.1.1` |
| `range-end` | string | Last allocatable IP (default: last host in network) | `10.100.255.254` |
| `gateway` | string | Pool-level gateway override | `10.100.0.1` |
| `dns` | array | Pool-level DNS override | `[192.168.100.10]` |
| `lease-time` | int | Pool-level lease time override (seconds) | `7200` |
| `priority` | int | Allocation priority; lower = tried first (default: 0) | `0` |
| `exclude` | array | IPs or ranges to exclude from allocation | `[10.100.0.2, 10.100.0.10-10.100.0.20]` |

The gateway IP is always excluded from allocation automatically.

## IP Allocation

When a subscriber session is created, the [provisioning pipeline](provisioning.md) determines the IP address:

1. If AAA returns an `ipv4_address`, that IP is used directly (pool is not consulted)
2. If AAA returns a `pool` attribute, that specific pool is tried first
3. Otherwise, pools in the profile are tried in priority order

All pool allocation is handled by a shared registry, so IPs are never double-allocated across IPoE and PPPoE.

In relay and proxy modes, pools are not used. The address comes from the upstream DHCP server.

## Examples

### Local Server

```yaml
ipv4-profiles:
  residential:
    gateway: 10.255.0.1
    dns:
      - 8.8.8.8
      - 8.8.4.4
    pools:
      - name: subscriber-pool
        network: 10.255.0.0/16
    dhcp:
      lease-time: 3600
```

### Relay

Forwards DHCP packets to an external server. osvbng sets the GIAddr, increments the hop count, and inserts Option 82.

```yaml
ipv4-profiles:
  relay-profile:
    gateway: 10.255.0.1
    dns:
      - 8.8.8.8
    dhcp:
      mode: relay
      servers:
        - address: "192.168.1.1:67"
          priority: 100
        - address: "192.168.1.2:67"
          priority: 50
      giaddr: 10.0.0.1
      option82:
        circuit-id-format: "{interface}:{svlan}:{cvlan}"
        remote-id-format: "{mac}"
```

### Relay with Passthrough Option 82

When the access node (DSLAM, OLT) already inserts Option 82, use `policy: keep` to preserve it.

```yaml
ipv4-profiles:
  relay-passthrough:
    gateway: 10.255.0.1
    dhcp:
      mode: relay
      servers:
        - address: "192.168.1.1:67"
          priority: 100
      giaddr: 10.0.0.1
      option82:
        policy: keep
```

### Proxy

Relays to an external server but presents osvbng as the DHCP server to the client. The client sees a shorter lease (`client-lease`) while osvbng maintains the full upstream lease.

When a client renews, the proxy answers locally without contacting the upstream server — as long as the upstream server's T1 (half the server lease) has not elapsed. Once T1 is reached, the next renewal is forwarded upstream to refresh the server-side lease. This significantly reduces upstream DHCP traffic while keeping clients on short renewal cycles.

For example, with `client-lease: 900` (15 minutes) and a 7-day upstream lease:
- Clients renew every ~7.5 minutes (T1 of the 15-minute client lease)
- The proxy answers these renewals locally
- After 3.5 days (T1 of the 7-day server lease), the proxy forwards the next renewal upstream

```yaml
ipv4-profiles:
  proxy-profile:
    gateway: 10.255.0.1
    dns:
      - 8.8.8.8
    dhcp:
      mode: proxy
      servers:
        - address: "192.168.1.1:67"
          priority: 100
      giaddr: 10.0.0.1
      client-lease: 900
      option82:
        circuit-id-format: "{interface}:{svlan}:{cvlan}"
        remote-id-format: "{mac}"
```

### Server Failover

Both relay and proxy modes support server failover. Servers are tried in priority order (highest first). After `dead-threshold` consecutive failures, a server is quarantined for `dead-time` before being retried.

```yaml
ipv4-profiles:
  resilient:
    gateway: 10.255.0.1
    dhcp:
      mode: relay
      servers:
        - address: "10.1.1.1:67"
          priority: 100
        - address: "10.1.1.2:67"
          priority: 50
      giaddr: 10.0.0.1
      server-timeout: 3s
      dead-time: 60s
      dead-threshold: 5
```
