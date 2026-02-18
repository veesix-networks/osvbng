# DHCP

DHCPv4 provider configuration. Address pools and delivery parameters are defined in [IPv4 profiles](ipv4-profiles.md), referenced by [subscriber groups](subscriber-groups.md) via `ipv4-profile`.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `provider` | string | DHCP provider: `local` | `local` |

## Example

```yaml
dhcp:
  provider: local
```
