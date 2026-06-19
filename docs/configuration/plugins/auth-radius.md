# subscriber.auth.radius <span class="version-badge">v0.5.0</span>

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
| `nas_port_id_format` | string | NAS-Port-Id template. See [placeholders](#nas-port-id-format) | `{interface}:{svlan}.{cvlan}` |
| `dead_time` | duration | How long to skip a dead server | `30s` |
| `dead_threshold` | int | Consecutive failures before marking dead | `3` |
| `vrf` | string | Default VRF for outbound auth/accounting traffic. Per-server `vrf` overrides this. | |
| `source_ip` | string | Default IPv4 source address for outbound auth/accounting. Per-server `source_ip` overrides this. | |
| `source_ipv6` | string | Default IPv6 source address for outbound auth/accounting. Per-server `source_ipv6` overrides this. | |
| `coa_listener` | [CoAListener](#coa-listener) | CoA/DM UDP listener settings (port, VRF, bind address). | |
| `coa_clients` | [CoAClient](#coa-client)[] | Authorized CoA senders. | |
| `coa_replay_window` | int | CoA Event-Timestamp replay window in seconds. Set to 0 to disable. | `300` |
| `response_mappings` | [ResponseMapping](#response-mappings)[] | Custom Tier 3 attribute mappings | |
| `request_mappings` | RequestMapping[] | Custom attributes added to Access-Request | |
| `accounting_mappings` | RequestMapping[] | Custom attributes added to Accounting-Request | |

### VRF and Source Address Cascade

`vrf` / `source_ip` / `source_ipv6` at the plugin level are the default control-plane binding for outbound traffic to every server. A per-[Server](#server) entry may override any of those fields individually. Fields are merged field-by-field, so setting `source_ip` on one server does not blank out an inherited `vrf`.

The CoA listener has its own binding ([CoAListener](#coa-listener)) and does not inherit from the plugin-level defaults. Incoming CoA arrives from RADIUS, not to it, so the cascade does not apply.

## Server

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `host` | string | RADIUS server hostname or IP | `10.1.1.1` |
| `secret` | string | Shared secret | `${RADIUS_SECRET}` |
| `vrf` | string | Override plugin-level `vrf` for this server. | `mgmt-vrf` |
| `source_ip` | string | Override plugin-level `source_ip` for this server (IPv4). | `10.0.0.1` |
| `source_ipv6` | string | Override plugin-level `source_ipv6` for this server (IPv6). | `2001:db8::1` |

Servers are tried in order. On timeout or error, the next server is attempted. After `dead_threshold` consecutive failures, a server is marked dead and skipped for `dead_time`.

The binding fields are merged with the plugin-level defaults field by field: a server can override a single field while inheriting the rest. An IPv6 `host` resolves source binding through `source_ipv6`; an IPv4 host through `source_ip`.

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
| Service-Type | 6 | `2` (Framed) for PPPoE/IPoE/L2TP, `5` (Outbound) otherwise |
| Calling-Station-Id | 31 | Subscriber MAC |
| NAS-Identifier | 32 | `nas_identifier` config |
| Called-Station-Id | 30 | Circuit ID (if available) |
| Acct-Session-Id | 44 | Session accounting ID |
| Event-Timestamp | 55 | Current time |
| NAS-Port | 5 | Access sub-interface sw_if_index |
| NAS-Port-Type | 61 | `nas_port_type` config |
| NAS-Port-Id | 87 | Formatted per `nas_port_id_format` |

### NAS-Port-Id format

`nas_port_id_format` builds NAS-Port-Id (attribute 87) from these placeholders. The same format is used for Access-Request and Accounting.

| Placeholder | Value |
|---|---|
| `{interface}` | Access sub-interface name, e.g. `eth1.100` (the `.100` is the sub-interface index, not the S-VLAN) |
| `{svlan}` | Outer (S-)VLAN tag |
| `{cvlan}` | Inner (C-)VLAN tag |
| `{subscriber_index}` | Subscriber dataplane sw_if_index. Accounting only; `0` in Access-Request (the subscriber interface is created after authentication) |

Default: `{interface}:{svlan}.{cvlan}` → e.g. `eth1.100:100.2`.

### Request Mappings

`request_mappings` adds attributes to the Access-Request, sourced from the subscriber's internal attributes. Each entry takes `internal` plus either a `radius_attr` name or a `vendor_id`/`vendor_type` pair. A mapping only emits when its `internal` attribute is set for that session.

Internal attributes available as sources:

| Internal | Source | Access types |
|---|---|---|
| `circuit_id` | Agent Circuit ID — DHCP Option 82 sub-option 1 / PPPoE TR-101 tag | IPoE, PPPoE |
| `remote_id` | Agent Remote ID — DHCP Option 82 sub-option 2 / PPPoE TR-101 tag | IPoE, PPPoE |
| `hostname` | Client hostname — DHCP Option 12 | IPoE |

```yaml
request_mappings:
  - internal: remote_id
    vendor_id: 3561
    vendor_type: 2
```

## Accounting

Accounting is automatic when the RADIUS provider is the active `auth_provider`. The AAA component calls Start/Interim-Update/Stop on session lifecycle events.

Accounting-Request packets include:

| AVP | Type | Description |
|-----|------|-------------|
| Acct-Status-Type | 40 | Start (1), Interim-Update (3), Stop (2) |
| Acct-Authentic | 45 | RADIUS (1) |
| Acct-Delay-Time | 41 | Seconds the record has been delayed (0) |
| Acct-Session-Id | 44 | Session accounting ID |
| User-Name | 1 | Subscriber username |
| Calling-Station-Id | 31 | Subscriber MAC |
| NAS-Identifier | 32 | NAS identifier |
| NAS-IP-Address | 4 | NAS IP |
| Called-Station-Id | 30 | Circuit ID (if available) |
| Service-Type | 6 | Framed (2) for subscriber access types |
| NAS-Port | 5 | Access sub-interface sw_if_index |
| NAS-Port-Type | 61 | From `nas_port_type` |
| NAS-Port-Id | 87 | Formatted per `nas_port_id_format` |
| Acct-Session-Time | 46 | Session duration (seconds) |
| Acct-Input-Octets | 42 | RX bytes (lower 32 bits) |
| Acct-Output-Octets | 43 | TX bytes (lower 32 bits) |
| Acct-Input-Gigawords | 52 | RX bytes (upper 32 bits), when non-zero |
| Acct-Output-Gigawords | 53 | TX bytes (upper 32 bits), when non-zero |
| Acct-Input-Packets | 47 | RX packets |
| Acct-Output-Packets | 48 | TX packets |
| Framed-IP-Address | 8 | Assigned IPv4 address |
| Framed-IPv6-Prefix | 97 | Assigned IPv6 WAN prefix |
| Delegated-IPv6-Prefix | 123 | Delegated IPv6 prefix |
| Event-Timestamp | 55 | Current time |

### Accounting Mappings

`accounting_mappings` adds vendor or standard attributes to Accounting-Request packets, sourced from the subscriber's internal attributes. Each entry takes `internal` plus either a `radius_attr` name or a `vendor_id`/`vendor_type` pair, the same as `request_mappings`. A mapping only emits when its `internal` attribute is set on the session.

```yaml
accounting_mappings:
  - internal: circuit_id
    vendor_id: 3561
    vendor_type: 1
  - internal: vrf
    vendor_id: 4874
    vendor_type: 1
```

## CoA / Disconnect-Message (RFC 5176)

osvbng can receive RADIUS Change of Authorization (CoA) and Disconnect-Message (DM) requests from authorized RADIUS servers. CoA changes subscriber attributes mid-session; Disconnect tears down a session.

CoA is implemented as a plugin component (`subscriber.auth.radius.coa`) that starts automatically when `coa_clients` is configured. It uses the event bus to communicate with subscriber components - no direct coupling.

If `coa_clients` is empty or absent, the CoA listener is not started. Top-level fields are documented in the [main config table](#subscriberauthradius-v050) above; this section describes the [CoA Listener](#coa-listener) and [CoA Client](#coa-client) sub-blocks.

### CoA Listener

`coa_listener` configures the UDP socket osvbngd binds to for incoming CoA/DM requests. It does not inherit the plugin-level auth/accounting binding, so the listener can live in a different VRF and on a different source address than outbound auth traffic.

| Field | Type | Description | Default |
|-------|------|-------------|---------|
| `port` | int | UDP port to listen for CoA/DM requests | `3799` |
| `vrf` | string | Linux VRF master to bind the listener to. | |
| `source_ip` | string | IPv4 address to bind the listener to. Empty = wildcard (`0.0.0.0`). | |

CoA-ACK / CoA-NAK / Disconnect-ACK responses are sent back over this same socket, so the reply path follows the listener's binding. There is no separate per-client reply binding.

### CoA Client

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `host` | string | IP address or CIDR range of the CoA sender | `10.1.1.1` or `10.244.0.0/16` |
| `secret` | string | Shared secret for this client | `${COA_SECRET}` |

The `host` field accepts bare IP addresses (matched as /32) or CIDR ranges. CIDR support is useful for Kubernetes deployments where CoA sources have ephemeral pod IPs within a known network:

```yaml
coa_clients:
  - host: "10.244.0.0/16"    # K8s pod CIDR
    secret: "${COA_SECRET}"
```

For environments where source IP filtering is not practical, use `0.0.0.0/0` to accept from any source. Message-Authenticator (HMAC-MD5) remains the authentication boundary:

```yaml
coa_clients:
  - host: "0.0.0.0/0"
    secret: "${COA_SECRET}"
```

### Supported Operations

**CoA-Request:** Changes subscriber session attributes. The request must contain at least one session identifier (Acct-Session-Id, User-Name, Framed-IP-Address, or Framed-IPv6-Address) and one or more mutable attributes. Attributes that require session teardown (IP addresses, VRF, pools) are rejected.

**Disconnect-Request:** Tears down a subscriber session. The request must contain only session identification attributes. The session is fully deprovisioned from the VPP dataplane.

### Error-Cause Values

| Code | Name | When |
|------|------|------|
| 201 | Residual Session Context Removed | Disconnect-ACK |
| 401 | Unsupported Attribute | CoA attribute not in the mutable set |
| 402 | Missing Attribute | No session identifier or no mutable attributes |
| 403 | NAS Identification Mismatch | NAS-Identifier doesn't match config |
| 404 | Invalid Request | Disconnect-Request contains non-identification attributes |
| 503 | Session Context Not Found | Target session not found |
| 506 | Resources Unavailable | Worker pool overflow or internal error |
| 507 | Request Initiated | Service-Type = Authorize Only (not supported) |

## Show Commands

| Path | API Endpoint | Description |
|------|-------------|-------------|
| `aaa.radius.servers` | `/api/show/aaa/radius/servers` | Per-server auth/acct statistics |
| `aaa.radius.coa` | `/api/show/aaa/radius/coa` | Per-client CoA/DM statistics |

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

### Full (with CoA)

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
    coa_listener:
      port: 3799
    coa_clients:
      - host: 10.1.1.1
        secret: "${COA_SECRET}"
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

### Per-VRF Servers

A common deployment: auth/accounting reach two RADIUS servers in different VRFs, and CoA is received on a separate management VRF. The plugin-level `vrf` and `source_ip` set the default for outbound traffic; each server overrides as needed. `coa_listener` is bound independently.

```yaml
plugins:
  subscriber.auth.radius:
    vrf: aaa-vrf
    source_ip: 10.0.0.1
    servers:
      - host: 10.1.1.1
        secret: "${RADIUS_SECRET_PRIMARY}"
      - host: 10.2.2.2
        secret: "${RADIUS_SECRET_SECONDARY}"
        vrf: aaa-vrf-backup
        source_ip: 10.0.1.1
    coa_listener:
      port: 3799
      vrf: mgmt-vrf
      source_ip: 192.0.2.10
    coa_clients:
      - host: 10.1.1.1
        secret: "${COA_SECRET}"
```
