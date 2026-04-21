# exporter.cgnat.http

Community HTTP exporter for CGNAT port-block allocation and release
events. Subscribes to `TopicCGNATMapping` on the internal event bus
and POSTs one JSON payload per allocate/release to a configured
endpoint. Intended primarily for **metadata retention and lawful-intercept
correlation** — the destination service persists the events and answers
"which subscriber had outside-IP:port at time T" queries.

The publisher is the CGNAT component; a single BNG can emit thousands
of port-block events per second at peak. The exporter consumes events
on a bounded in-memory queue with a dedicated worker pool so the
mapping hot path never blocks on HTTP I/O.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `enabled` | bool | Enable the plugin | `true` |
| `endpoint` | string | Destination URL for each event | `https://portal.example.com/api/v1/bng/cgnat-mapping` |
| `method` | string | HTTP method (default `POST`) | `POST` |
| `timeout` | duration | Per-request timeout (default `5s`) | `5s` |
| `tls` | object | TLS configuration | |
| `auth` | object | HTTP authentication | |
| `headers` | map | Additional request headers | |
| `queue_size` | int | In-memory queue capacity (default `10000`) | `10000` |
| `workers` | int | Concurrent HTTP workers (default `1`) | `2` |
| `max_retries` | int | Retry attempts after the first POST fails (default `3`) | `5` |
| `retry_initial` | duration | Initial backoff between retries (default `500ms`) | `500ms` |
| `retry_max` | duration | Maximum backoff (default `30s`) | `30s` |
| `include_inside_ip` | bool | Include the subscriber's inside IP in the payload (default `true`) | `true` |

## TLS

Same shape as the `subscriber.auth.http` plugin:

| Field | Type | Description |
|-------|------|-------------|
| `insecure_skip_verify` | bool | Skip TLS certificate verification |
| `ca_cert_file` | string | Path to a CA certificate PEM file |
| `cert_file` | string | Path to a client certificate PEM file |
| `key_file` | string | Path to a client private key PEM file |

## Auth

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `type` | string | `basic` or `bearer` | `bearer` |
| `username` | string | Basic-auth username | `admin` |
| `password` | string | Basic-auth password | |
| `token` | string | Bearer token | |

## Payload

Each event is POSTed as a single JSON object:

```json
{
  "event": "allocate",
  "at": "2026-04-20T08:01:12.345Z",
  "srg_name": "default",
  "session_id": "f6be89db-7454-41fb-9849-fc4aa683a9a6",
  "pool_name": "cgnat-syd-01",
  "pool_id": 7,
  "outside_ip": "100.64.12.7",
  "port_block_start": 49152,
  "port_block_end": 49351,
  "inside_ip": "10.50.14.9",
  "inside_vrf_id": 42
}
```

`event` is `allocate` for new port-block assignments and `release`
when a block is returned to the pool. `session_id` correlates to the
BNG session (same value emitted on the auth/accounting endpoints of
`subscriber.auth.http`), so the downstream service can join
port-block events to subscriber identity via its own records.

## Reliability

- **Queue overflow** — events arriving when the internal queue is full are
  **dropped** (counted), never blocked. The publisher is the CGNAT
  component's mapping hot path; blocking there would slow the entire
  dataplane. Operators should alert on a non-zero drop counter.
- **HTTP retries** — network errors and 5xx responses are retried with
  exponential backoff up to `max_retries`. 4xx responses (client errors
  — bad payload, bad auth) are **not** retried; the event is recorded
  as failed so the operator is prompted to fix the configuration.
- **Shutdown** — on component stop, the subscriber is removed from the
  bus and the queue is drained for up to 5 seconds. Events still
  queued after the grace period are lost.

!!! warning "At-most-once, not at-least-once"
    This exporter makes a best-effort to deliver every event but the
    in-memory queue + no disk spooling means a BNG process crash or a
    sustained portal outage will lose events. For strict compliance
    regimes that require at-least-once delivery, pair this plugin with
    a durable collector (local syslog, Kafka, etc.) on the BNG.

## Example

```yaml
plugins:
  exporter.cgnat.http:
    enabled: true
    endpoint: https://portal.example.com/api/v1/bng/cgnat-mapping
    method: POST
    timeout: 5s
    queue_size: 10000
    workers: 2
    max_retries: 5
    retry_initial: 500ms
    retry_max: 30s
    include_inside_ip: true
    auth:
      type: bearer
      token: REPLACE_WITH_PORTAL_TOKEN
    headers:
      X-BNG-Node-Id: "osvbng-nsw-1"
```
