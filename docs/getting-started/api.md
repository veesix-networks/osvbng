# Northbound API

osvbng includes a REST API that auto-generates endpoints from registered [show, config, and oper handlers](../architecture/HANDLERS.md). Every `show` handler becomes a GET endpoint under `/api/show/`, every `conf` handler becomes a POST endpoint under `/api/set/`, and every `oper` handler becomes a POST endpoint under `/api/exec/`.

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
| GET | `/api/show/{path}` | Execute show handler |
| POST | `/api/set/{path}` | Execute config handler |
| POST | `/api/exec/{path}` | Execute oper handler |

## Response shapes

`GET /api/show/{path}` returns one of two envelope shapes depending on what the
handler returns:

- **Single-object handlers** (stats, status, version, single-record lookups,
  e.g. `show.subscriber.session`, `show.dhcp.relay`, `show.system.version`)
  return `{ "path": "...", "data": <object> }`. `data` is the handler's full
  return value. **No pagination.**
- **List-returning handlers** (e.g. `show.subscriber.sessions`,
  `show.cgnat.sessions`, `show.system.opdb.sessions`, `show.interfaces`)
  are **paginated by default** and return
  `{ "path": "...", "data": [<items>], "pagination": <Pagination> }`. `data`
  is always an array; `pagination` is the metadata block described below.

The OpenAPI spec at `/api/openapi.json` reflects both shapes — paginated
operations advertise the `limit`/`offset` query parameters and reference the
reusable `#/components/schemas/Pagination` schema, while single-object
operations carry no pagination params.

## Pagination

List-returning `show` endpoints accept two query parameters:

| Param | Type | Default | Cap | Meaning |
|---|---|---|---|---|
| `limit` | integer | `100` | server-clamped to `1000` | maximum items in this page |
| `offset` | integer | `0` | — | items to skip from the start of the sorted result |

The response envelope includes a `pagination` block:

```json
{
  "path": "subscriber.sessions",
  "data": [ /* up to limit items */ ],
  "pagination": {
    "limit": 100,
    "offset": 400,
    "returned": 100,
    "total": 12345,
    "has_more": true
  }
}
```

| Field | Meaning |
|---|---|
| `limit` | Page size used for this response (after clamping). |
| `offset` | Echo of the request offset. |
| `returned` | Actual items returned in `data`; may be less than `limit` on the last page. |
| `total` | Total items in the (post-handler-filter) result set. |
| `has_more` | `true` when more items exist beyond this page (`offset + returned < total`). |

### Sort order

Each list-returning handler picks a deterministic sort key (e.g. subscriber
sessions sort by `SessionID`, CGNAT mappings by `outside_ip`, interfaces by
`name`). This means `?offset=400&limit=100` returns the same block on
consecutive calls **as long as the underlying set has not changed**.

### Offset stability caveat

Offset pagination is not snapshot-isolated. If items are added or removed
between two calls, the offsets shift — `?offset=400&limit=100` on call N may
overlap or skip one item versus call N+1, in proportion to mutations that
landed before `offset`. For operator polling at typical rates this is invisible;
for an exact one-pass walk over a churning dataset, restart from `offset=0` if
the totals drift.

### Pagination examples

Walk a 12 345-item subscriber table page-by-page:

```bash
curl 'http://localhost:8080/api/show/subscriber/sessions?limit=100&offset=0'
curl 'http://localhost:8080/api/show/subscriber/sessions?limit=100&offset=100'
# … until "has_more": false
```

Force a small first page (Swagger UI scale safety):

```bash
curl 'http://localhost:8080/api/show/subscriber/sessions?limit=10'
```

Request more than the cap — server clamps and returns 1000 items:

```bash
curl 'http://localhost:8080/api/show/subscriber/sessions?limit=50000'
# response: pagination.limit == 1000
```

## Path Mapping

Handler paths use dots internally (e.g., `subscriber.sessions`), API paths use slashes:

| Handler Path | API Endpoint |
|--------------|--------------|
| `subscriber.sessions` | `GET /api/show/subscriber/sessions` |
| `interfaces` | `GET /api/show/interfaces` |
| `interfaces.eth1.description` | `POST /api/set/interfaces/eth1/description` |
| `system.logging.level.<*>` | `POST /api/exec/system/logging/level/{name}` |

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
  "oper_paths": [
    "system.logging.level.<*>"
  ]
}
```

### Get Running Configuration

```bash
curl http://localhost:8080/api/running-config
```

### Show Subscriber Sessions (paginated)

```bash
curl 'http://localhost:8080/api/show/subscriber/sessions?limit=2'
```

```json
{
  "path": "subscriber.sessions",
  "data": [
    {
      "SessionID": "abc123",
      "MAC": "00:11:22:33:44:55",
      "IPv4Address": "10.100.1.50",
      "IPv6Address": "2001:db8::1",
      "OuterVLAN": 100,
      "InnerVLAN": 10
    },
    {
      "SessionID": "abc124",
      "MAC": "00:11:22:33:44:56",
      "IPv4Address": "10.100.1.51",
      "IPv6Address": "2001:db8::2",
      "OuterVLAN": 100,
      "InnerVLAN": 11
    }
  ],
  "pagination": {
    "limit": 2,
    "offset": 0,
    "returned": 2,
    "total": 50,
    "has_more": true
  }
}
```

### Show Single Session

```bash
curl "http://localhost:8080/api/show/subscriber/session?session_id=abc123"
```

Query parameters are passed as options to the handler.

### Update Interface Description

```bash
curl -X POST http://localhost:8080/api/set/interfaces/eth1/description \
  -H "Content-Type: application/json" \
  -d '"New description"'
```

```json
{
  "status": "ok"
}
```

### Set Log Level (Oper Command)

```bash
curl -X POST http://localhost:8080/api/exec/system/logging/level/ipoe.dhcp4 \
  -H "Content-Type: application/json" \
  -d '{"level": "debug"}'
```

```json
{
  "name": "ipoe.dhcp4",
  "level": "debug"
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
