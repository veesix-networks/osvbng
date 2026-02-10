# Monitoring

Metrics collection configuration.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `disabled_collectors` | array | Collectors to disable | `[memory]` |
| `collect_interval` | duration | Collection interval | `10s` |

## Example

```yaml
monitoring:
  collect_interval: 10s
  disabled_collectors:
    - memory
```
