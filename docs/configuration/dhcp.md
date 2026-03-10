# DHCP

DHCPv4 provider configuration. The `provider` field selects the top-level DHCP engine (`local`). Relay and proxy modes are configured per-profile in [IPv4 profiles](ipv4-profiles.md) via the `dhcp.mode` field.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `provider` | string | DHCP provider: `local` | `local` |

## Modes

osvbng supports three DHCPv4 modes, selected per IPv4 profile:

| Mode | Description |
|------|-------------|
| `server` | Local DHCP server, allocates addresses from configured pools (default) |
| `relay` | RFC 2131 relay agent, forwards DHCP packets to external servers with Option 82 insertion |
| `proxy` | DHCP proxy, relays to external servers but manages client-facing leases independently |

Mode is configured under `ipv4-profiles.<name>.dhcp.mode`. See [IPv4 Profiles](ipv4-profiles.md#dhcp-options) for the full configuration reference.

## Example

```yaml
dhcp:
  provider: local
```

See [IPv4 Profiles](ipv4-profiles.md) for relay and proxy configuration examples and [DHCP Relay & Proxy](../architecture/DHCP_RELAY_PROXY.md) for architecture details.
