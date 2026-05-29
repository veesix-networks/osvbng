# AAA

Authentication, Authorization, and Accounting configuration.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `auth_provider` | string | AAA provider: `local`, `http`, `radius` | `local` |
| `nas_identifier` | string | NAS identifier string | `osvbng` |
| `nas_ip` | string | NAS IP address | `10.255.0.1` |
| `policy` | [AAAPolicy](#aaa-policies) | AAA policies | |

## AAA Policies

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `name` | string | Policy name | `default-policy` |
| `format` | string | Username format | `$mac-address$` |
| `password` | string | Placeholder password sent on DHCP/IPoE RADIUS Access-Request as `User-Password` (RFC 2865 attribute 2). Supports the same expansion variables as `format`. Unset = no `User-Password` attribute on the wire. | `example` |
| `type` | string | Session type: `dhcp` or `ppp` | `dhcp` |
| `authenticate` | bool | Validate PPP credentials (CHAP/PAP). Default `false` | `false` |
| `max_concurrent_sessions` | int | Max sessions per subscriber | `1` |

When `authenticate` is `false` (default), the subscriber is identified by the policy `format` field only. PPP CHAP/PAP handshakes complete at the protocol level but credentials are not validated against the auth provider. The subscriber is authorized if the user exists and is enabled.

When `authenticate` is `true`, the auth provider validates CHAP/PAP credentials. The user must have a password configured in the auth provider database.

### DHCP/IPoE Placeholder Password

DHCP has no subscriber-supplied credential. RADIUS servers that key on `User-Password` (e.g. FreeRADIUS `Cleartext-Password` check items) need an attribute on the Access-Request to match against. The `password` field on the policy is copied into `User-Password` for every DHCP/IPoE RADIUS Access-Request published by IPoE DHCPv4/DHCPv6 handlers. PPP-driven flows (PPPoE, L2TP LNS) ignore this field — they use the credentials negotiated during PAP/CHAP.

The value is treated as opaque by osvbng: it is not validated, only transported. Operators typically configure a constant matching what is provisioned in the RADIUS server, or `$remote-id$` to drive per-subscriber check items keyed on the relay tag.

FreeRADIUS pairing:

```
# /etc/freeradius/3.0/users
DEFAULT Cleartext-Password := "example"
        Framed-Pool := "subscriber-pool"
```

## Username Format Variables

| Variable | Description |
|----------|-------------|
| `$mac-address$` | Subscriber MAC address |
| `$svlan$` | S-VLAN ID |
| `$cvlan$` | C-VLAN ID |
| `$circuit-id$` | DHCP Option 82 Circuit ID |
| `$remote-id$` | DHCP Option 82 Remote ID |
| `$agent-circuit-id$` | Agent Circuit ID |
| `$agent-remote-id$` | Agent Remote ID |
| `$agent-relay-id$` | Agent Relay ID |
| `$hostname$` | Subscriber hostname |

## Example

```yaml
aaa:
  auth_provider: local
  nas_identifier: osvbng
  policy:
    - name: default-policy
      format: "$mac-address$"
      max_concurrent_sessions: 1
    - name: credential-policy
      format: "$mac-address$"
      authenticate: true
      max_concurrent_sessions: 1
    - name: radius-dhcp-policy
      format: "$remote-id$"
      password: "example"
      type: dhcp
      max_concurrent_sessions: 1
```
