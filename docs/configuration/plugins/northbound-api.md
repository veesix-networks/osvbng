# northbound.api

REST API for management and operational queries.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `enabled` | bool | Enable the northbound REST API | `true` |
| `listen_address` | string | Address and port to listen on | `:8080` |

## Example

```yaml
plugins:
  northbound.api:
    enabled: true
    listen_address: ":8080"
```
