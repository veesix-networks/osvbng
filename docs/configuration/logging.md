# Logging

Controls log output format and verbosity.

| Field | Type | Default | Description | Example |
|-------|------|---------|-------------|---------|
| `format` | string | `text` | Output format: `text` or `json` | `json` |
| `level` | string | `info` | Global log level: `debug`, `info`, `warn`, `error` | `info` |
| `components` | [Components](#components) | | Per-component log levels | |

## Components

Available components for per-component log levels:

| Component | Description |
|-----------|-------------|
| `main` | Main application |
| `aaa` | AAA (authentication, authorization, accounting) |
| `ipoe` | IPoE sessions |
| `pppoe` | PPPoE sessions |
| `l2tp` | L2TPv2 tunnels and sessions (LAC and LNS) |
| `l2gw` | Layer 2 wholesale gateway |
| `arp` | ARP handling |
| `dataplane` | Dataplane management |
| `subscriber` | Subscriber management |
| `routing` | Routing protocols |
| `southbound` | Southbound interface |
| `egress` | Egress processing |
| `events` | Event system |
| `srg` | Session Redundancy Group |
| `ha` | High availability |
| `cgnat` | CGNAT |
| `watchdog` | Watchdog and health endpoints |
| `bootstrap` | Bootstrap/startup |
| `monitor` | Monitoring |
| `confmgr` | Configuration manager |
| `gateway` | Gateway |
| `nb` | Northbound interface |
| `svcgroup` | Service groups |
| `telemetry` | Telemetry registry |
| `dhcp4` | IPoE DHCPv4 |
| `dhcp6` | IPoE DHCPv6 |
| `session` | IPoE session handling |
| `relay` | IPoE relay |

## Example

```yaml
logging:
  format: text
  level: info
  components:
    ipoe: debug
    aaa: info
    dataplane: warn
```
