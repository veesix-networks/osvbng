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

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `mode` | string | Profile mode (default: `server`) | `server` |
| `address-model` | string | `connected-subnet` (default) or `unnumbered-ptp` | `connected-subnet` |
| `server-id` | string | DHCP server identifier; defaults to gateway | `10.255.0.1` |
| `lease-time` | int | Default lease time in seconds (default: 3600) | `3600` |

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

## Example

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
      - name: overflow-pool
        network: 10.254.0.0/16
        gateway: 10.254.0.1
        priority: 10
        exclude:
          - 10.254.0.2-10.254.0.10
    dhcp:
      lease-time: 3600

  business:
    gateway: 172.16.0.1
    dns:
      - 1.1.1.1
      - 1.0.0.1
    pools:
      - name: business-pool
        network: 172.16.0.0/22
    dhcp:
      address-model: unnumbered-ptp
      server-id: 172.16.0.1
      lease-time: 86400
```
