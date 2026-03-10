# DHCP Relay and Proxy

osvbng supports DHCPv4 and DHCPv6 relay and proxy modes as certified provider plugins. Relay forwards DHCP packets to external servers. Proxy does the same but manages client-facing leases independently, presenting osvbng as the DHCP server to the subscriber.

Both modes support server failover with priority-based selection and automatic dead-server quarantine.

## Relay Mode

### DHCPv4

The relay agent:

1. Sets the GIAddr field to the configured relay IP
2. Forwards to the highest-priority non-dead server
3. Returns the server reply to the client

RELEASE messages are forwarded but no reply is expected.

### DHCPv6

The relay agent:

1. Wraps the client message in a Relay-Forward envelope
2. Forwards to the highest-priority non-dead server
3. Returns the server reply to the client

## Proxy Mode

Proxy mode extends relay with lease/binding management. The proxy appears as the DHCP server to the client while maintaining a separate upstream lease.

### DHCPv4

1. On DISCOVER: forwards to server, stores the server's Server-ID in the binding
2. On OFFER reply: rewrites Server-ID to GIAddr, sets lease time to `client-lease`, adjusts T1/T2
3. On REQUEST: restores the original Server-ID from binding before forwarding (the server would silently drop the packet if the Server-ID doesn't match)
4. On ACK reply: stores full binding (client IP, server IP, server lease, timestamps), rewrites response for client
5. On RELEASE: forwards to server, deletes binding

### DHCPv6

1. On Solicit: forwards to server, stores the server's DUID in the binding
2. On Advertise reply: replaces Server-ID (DUID) with proxy's own generated DUID, rewrites all IA_NA/IA_PD lifetimes to client values
3. On Request/Renew/Rebind: restores original Server DUID from binding before forwarding
4. On Reply: stores full binding, rewrites lifetimes and Server-ID for client
5. On Release: forwards to server, deletes binding

The proxy generates its own DUID at startup. DHCPv4 bindings are keyed by MAC address, DHCPv6 bindings by DUID.

## Server Failover

The relay client tracks per-server health:

| Field | Description |
|-------|-------------|
| `priority` | Servers are tried in priority order (highest first) |
| `dead-threshold` | Consecutive timeouts before marking a server dead |
| `dead-time` | How long a dead server stays quarantined before being retried |
| `server-timeout` | Time to wait for a server response |

When a request times out, the server's failure counter increments. After `dead-threshold` consecutive failures, the server is marked dead and skipped for `dead-time`. A successful response immediately clears the dead state and resets the failure counter.

If all servers are dead, requests fail with an error until at least one server recovers.

## Show Commands

| API Path | CLI Path | Description |
|----------|----------|-------------|
| `GET /api/show/dhcp/relay` | `show dhcp relay` | Relay statistics and per-server status |
| `GET /api/show/dhcp/proxy` | `show dhcp proxy` | Active binding counts |

### Relay Output

```json
{
  "data": {
    "stats": {
      "requests4": 1000,
      "replies4": 999,
      "timeouts4": 1,
      "requests6": 500,
      "replies6": 499,
      "timeouts6": 1
    },
    "servers": [
      {
        "address": "192.168.1.1:67",
        "priority": 100,
        "dead": false,
        "failures": 0,
        "requests": 1000,
        "timeouts": 1
      }
    ]
  }
}
```

### Proxy Output

```json
{
  "data": {
    "v4Bindings": 1000,
    "v6Bindings": 500
  }
}
```

## Prometheus Metrics

All metrics are exported via the Prometheus exporter plugin.

### Relay Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `osvbng_dhcp_relay_requests_v4` | counter | DHCPv4 requests forwarded |
| `osvbng_dhcp_relay_replies_v4` | counter | DHCPv4 replies received |
| `osvbng_dhcp_relay_timeouts_v4` | counter | DHCPv4 server timeouts |
| `osvbng_dhcp_relay_requests_v6` | counter | DHCPv6 requests forwarded |
| `osvbng_dhcp_relay_replies_v6` | counter | DHCPv6 replies received |
| `osvbng_dhcp_relay_timeouts_v6` | counter | DHCPv6 server timeouts |
| `osvbng_dhcp_relay_server_requests` | counter | Per-server request count (labelled by address) |
| `osvbng_dhcp_relay_server_timeouts` | counter | Per-server timeout count (labelled by address) |

### Proxy Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `osvbng_dhcp_proxy_bindings_v4` | gauge | Active DHCPv4 proxy bindings |
| `osvbng_dhcp_proxy_bindings_v6` | gauge | Active DHCPv6 proxy bindings |

