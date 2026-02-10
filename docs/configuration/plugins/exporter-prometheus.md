# exporter.prometheus

Prometheus metrics exporter. Exposes an HTTP endpoint for Prometheus to scrape.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `enabled` | bool | Enable the Prometheus metrics exporter | `true` |
| `listen_address` | string | Address and port to listen on for scrape requests | `:9090` |

## Example

```yaml
plugins:
  exporter.prometheus:
    enabled: true
    listen_address: ":9090"
```
