# AAA

Authentication, Authorization, and Accounting configuration.

| Field | Type | Description |
|-------|------|-------------|
| `provider` | string | AAA provider: `local` |
| `nas_identifier` | string | NAS identifier string |
| `nas_ip` | string | NAS IP address |
| `policy` | array | AAA policies |

## AAA Policies

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Policy name |
| `format` | string | Username format |
| `type` | string | Session type: `dhcp` or `pppoe` |
| `max_concurrent_sessions` | int | Max sessions per subscriber |

## Username Format Variables

| Variable | Description |
|----------|-------------|
| `$mac-address$` | Subscriber MAC address |
| `$circuit-id$` | DHCP Option 82 Circuit ID |
| `$remote-id$` | DHCP Option 82 Remote ID |

## Example

```yaml
aaa:
  provider: local
  nas_identifier: osvbng
  policy:
    - name: default-policy
      format: "$mac-address$"
      type: dhcp
      max_concurrent_sessions: 1
```
