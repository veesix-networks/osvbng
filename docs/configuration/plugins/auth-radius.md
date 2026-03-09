# subscriber.auth.radius

Authentication and accounting client for RADIUS servers. Sends Access-Request packets, translates Access-Accept AVPs into internal subscriber attributes, and handles accounting (Start/Interim-Update/Stop).

Supports ordered server failover with dead server detection and a three-tier attribute mapping system.

| Field | Type | Description | Default |
|-------|------|-------------|---------|
| `servers` | [Server](#server)[] | Ordered list of RADIUS servers | *required* |
| `auth_port` | int | Authentication port | `1812` |
| `acct_port` | int | Accounting port | `1813` |
| `timeout` | duration | Per-attempt timeout | `3s` |
| `retries` | int | Per-server retry count | `3` |
| `nas_identifier` | string | NAS-Identifier AVP. Falls back to `aaa.nas_identifier` | |
| `nas_ip` | string | NAS-IP-Address AVP. Falls back to `aaa.nas_ip` | |
| `nas_port_type` | string | NAS-Port-Type AVP value | `Virtual` |
| `dead_time` | duration | How long to skip a dead server | `30s` |
| `dead_threshold` | int | Consecutive failures before marking dead | `3` |
| `response_mappings` | [ResponseMapping](#response-mappings)[] | Custom Tier 3 attribute mappings | |

## Server

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `host` | string | RADIUS server hostname or IP | `10.1.1.1` |
| `secret` | string | Shared secret | `${RADIUS_SECRET}` |

Servers are tried in order. On timeout or error, the next server is attempted. After `dead_threshold` consecutive failures, a server is marked dead and skipped for `dead_time`.

## Attribute Mapping

The provider uses a three-tier attribute mapping system. All tiers are evaluated on every Access-Accept response.

### Tier 1 — RFC Standard (always active)

| RADIUS AVP | Type | Internal Attribute |
|---|---|---|
| Framed-IP-Address | 8 | `ipv4_address` |
| Framed-IP-Netmask | 9 | `ipv4_netmask` |
| Framed-Route | 22 | `routed_prefix` |
| Session-Timeout | 27 | `session_timeout` |
| Idle-Timeout | 28 | `idle_timeout` |
| Acct-Interim-Interval | 85 | `acct_interim_interval` |
| Framed-Pool | 88 | `pool` |
| Framed-IPv6-Prefix | 97 | `ipv6_wan_prefix` |
| Framed-IPv6-Pool | 100 | `iana_pool` |
| Delegated-IPv6-Prefix | 123 | `ipv6_prefix` |
| Framed-IPv6-Address | 168 | `ipv6_address` |
| Delegated-IPv6-Prefix-Pool | 171 | `pd_pool` |

### Tier 2 — Common Vendor Defaults (always active)

| Vendor | Vendor ID | Attribute | Type | Internal Attribute |
|---|---|---|---|---|
| Microsoft | 311 | MS-Primary-DNS-Server | 28 | `dns_primary` |
| Microsoft | 311 | MS-Secondary-DNS-Server | 29 | `dns_secondary` |

Additional vendor attributes will be added over time based on deployment feedback. Use [Tier 3 custom mappings](#response-mappings) for vendor-specific attributes not yet covered.

### Tier 3 — Custom Mappings (config-driven)

For vendor-specific attributes not covered by Tier 1 or 2, define custom mappings using raw `vendor_id` and `vendor_type` integers.

## Response Mappings

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `vendor_id` | int | RADIUS vendor ID | `9` (Cisco) |
| `vendor_type` | int | Vendor sub-attribute type | `1` |
| `internal` | string | Internal attribute name to set | `vrf` |
| `extract` | string | Optional regex with capture group | `ip:vrf-id=(.+)` |

When `extract` is set, the VSA string value is matched against the regex and the first capture group is used as the attribute value. Without `extract`, the full VSA string value is used.

!!! tip "Finding vendor IDs and types"
    Use FreeRADIUS dictionary files as a reference for vendor ID and attribute type numbers. The `layeh/radius` project provides a [`radius-dict-gen`](https://github.com/layeh/radius) tool for converting dictionaries to Go code if needed for development.

## Access-Request AVPs

The following AVPs are included in every Access-Request:

| AVP | Type | Source |
|-----|------|--------|
| User-Name | 1 | AAA policy username |
| User-Password / CHAP-Password | 2/3 | Subscriber credentials |
| NAS-IP-Address | 4 | `nas_ip` config |
| Service-Type | 6 | `2` (Framed) for PPPoE, `5` (Outbound) for IPoE |
| Calling-Station-Id | 31 | Subscriber MAC |
| NAS-Identifier | 32 | `nas_identifier` config |
| Called-Station-Id | 30 | Circuit ID (if available) |
| Acct-Session-Id | 44 | Session accounting ID |
| Event-Timestamp | 55 | Current time |
| NAS-Port-Type | 61 | `nas_port_type` config |
| NAS-Port-Id | 87 | Subscriber interface |

## Accounting

Accounting is automatic when the RADIUS provider is the active `auth_provider`. The AAA component calls Start/Interim-Update/Stop on session lifecycle events.

Accounting-Request packets include:

| AVP | Type | Description |
|-----|------|-------------|
| Acct-Status-Type | 40 | Start (1), Interim-Update (3), Stop (2) |
| Acct-Session-Id | 44 | Session accounting ID |
| User-Name | 1 | Subscriber username |
| Calling-Station-Id | 31 | Subscriber MAC |
| NAS-Identifier | 32 | NAS identifier |
| NAS-IP-Address | 4 | NAS IP |
| Acct-Session-Time | 46 | Session duration (seconds) |
| Acct-Input-Octets | 42 | RX bytes |
| Acct-Output-Octets | 43 | TX bytes |
| Acct-Input-Packets | 47 | RX packets |
| Acct-Output-Packets | 48 | TX packets |
| Framed-IP-Address | 8 | Assigned IPv4 address |
| Event-Timestamp | 55 | Current time |

## Show Commands

| Path | API Endpoint | Description |
|------|-------------|-------------|
| `aaa.radius.servers` | `/api/show/aaa/radius/servers` | Per-server auth/acct statistics |

## Example

### Minimal

```yaml
aaa:
  auth_provider: radius
  nas_identifier: osvbng

plugins:
  subscriber.auth.radius:
    servers:
      - host: 10.1.1.1
        secret: testing123
    nas_ip: 10.0.0.1
```

### Full

```yaml
aaa:
  auth_provider: radius
  nas_identifier: osvbng

plugins:
  subscriber.auth.radius:
    servers:
      - host: 10.1.1.1
        secret: "${RADIUS_SECRET_PRIMARY}"
      - host: 10.1.1.2
        secret: "${RADIUS_SECRET_SECONDARY}"
    auth_port: 1812
    acct_port: 1813
    timeout: 3s
    retries: 3
    nas_ip: 10.0.0.1
    nas_port_type: Virtual
    dead_time: 30s
    dead_threshold: 3
    response_mappings:
      - vendor_id: 9
        vendor_type: 1
        internal: vrf
        extract: "ip:vrf-id=(.+)"
      - vendor_id: 9
        vendor_type: 1
        internal: qos.egress-policy
        extract: "sub-qos-policy-out=(.+)"
      - vendor_id: 9
        vendor_type: 1
        internal: qos.ingress-policy
        extract: "sub-qos-policy-in=(.+)"
```
