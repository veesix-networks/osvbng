# Northbound API

osvbng includes a REST API that auto-generates endpoints from registered [show and config handlers](../architecture/HANDLERS.md). Every `show` handler becomes a GET endpoint, and every `conf` handler becomes a POST endpoint.

!!! success "Enabled by default"
    The northbound API is enabled by default on port `8080`. No configuration needed.

## Configuration

```yaml
plugins:
  northbound.api:
    enabled: true
    listen_address: ":8080"
```

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api` | List all available paths |
| GET | `/api/running-config` | Get running configuration |
| GET | `/api/startup-config` | Get startup configuration |
| GET | `/api/{path}` | Execute show handler |
| POST | `/api/{path}` | Execute config handler |

## Path Mapping

Handler paths use dots internally (e.g., `subscriber.sessions`), API paths use slashes:

| Handler Path | API Endpoint |
|--------------|--------------|
| `subscriber.sessions` | `GET /api/subscriber/sessions` |
| `interfaces` | `GET /api/interfaces` |
| `interfaces.eth1.description` | `POST /api/interfaces/eth1/description` |

## Examples

### List Available Endpoints

```bash
curl http://localhost:8080/api
```

```json
{
  "show_paths": [
    "subscriber.sessions",
    "subscriber.session",
    "interfaces",
    "bgp.summary"
  ],
  "config_paths": [
    "interfaces.*.description",
    "interfaces.*.enabled"
  ],
  "oper_paths": []
}
```

### Get Running Configuration

```bash
curl http://localhost:8080/api/running-config
```

### Show Subscriber Sessions

```bash
curl http://localhost:8080/api/subscriber/sessions
```

```json
{
  "path": "subscriber.sessions",
  "data": {
    "sessions": [
      {
        "session_id": "abc123",
        "mac": "00:11:22:33:44:55",
        "ip": "10.100.1.50",
        "svlan": 100,
        "cvlan": 10
      }
    ]
  }
}
```

### Show Single Session

```bash
curl "http://localhost:8080/api/subscriber/session?session_id=abc123"
```

Query parameters are passed as options to the handler.

### Update Interface Description

```bash
curl -X POST http://localhost:8080/api/interfaces/eth1/description \
  -H "Content-Type: application/json" \
  -d '"New description"'
```

```json
{
  "status": "ok"
}
```

## Error Handling

Errors return JSON with an `error` field:

```json
{
  "error": "session not found"
}
```

| Status Code | Meaning |
|-------------|---------|
| 200 | Success |
| 400 | Bad request (invalid path or body) |
| 500 | Internal error (handler failed) |
