# Logging

Controls log output format and verbosity.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `format` | string | `text` | Output format: `text` or `json` |
| `level` | string | `info` | Global log level: `debug`, `info`, `warn`, `error` |
| `components` | map | | Per-component log levels |

## Components

Available components for per-component log levels:

| Component | Description |
|-----------|-------------|
| `ipoed` | IPoE daemon |
| `aaad` | AAA daemon |
| `dhcprd` | DHCP relay daemon |
| `dpd` | Dataplane daemon |
| `subd` | Subscriber daemon |
| `arpd` | ARP daemon |
| `routerd` | Routing daemon |
| `sb` | Southbound interface |

## Example

```yaml
logging:
  format: text
  level: info
  components:
    ipoed: debug
    aaad: info
    dhcprd: warn
```
