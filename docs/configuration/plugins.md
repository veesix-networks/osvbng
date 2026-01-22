# Plugins

Plugin-specific configuration. Each key under `plugins` is a plugin name with its own configuration.

## Available Plugins

### subscriber.auth.local

Local subscriber authentication.

| Field | Type | Description |
|-------|------|-------------|
| `database_path` | string | SQLite database path |
| `allow_all` | bool | Allow all subscribers without authentication |

### exporter.prometheus

Prometheus metrics exporter.

| Field | Type | Description |
|-------|------|-------------|
| `enabled` | bool | Enable the exporter |
| `listen_address` | string | Listen address (e.g., `:9090`) |

### northbound.api

REST API for management.

| Field | Type | Description |
|-------|------|-------------|
| `enabled` | bool | Enable the API |
| `listen_address` | string | Listen address (e.g., `:8080`) |

## Example

```yaml
plugins:
  subscriber.auth.local:
    database_path: /var/lib/osvbng/subscribers.db
    allow_all: false

  exporter.prometheus:
    enabled: true
    listen_address: ":9090"

  northbound.api:
    enabled: true
    listen_address: ":8080"
```
