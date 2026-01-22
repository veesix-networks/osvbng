# Monitoring

Metrics collection configuration.

| Field | Type | Description |
|-------|------|-------------|
| `disabled_collectors` | array | Collectors to disable |
| `collect_interval` | duration | Collection interval (e.g., `10s`) |

## Example

```yaml
monitoring:
  collect_interval: 10s
  disabled_collectors:
    - memory
```
