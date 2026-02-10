# subscriber.auth.http

HTTP-based subscriber authentication. Authenticates subscribers against an external HTTP API.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `endpoint` | string | URL of the authentication endpoint | `https://auth.example.com/api/auth` |
| `method` | string | HTTP method | `POST` |
| `timeout` | duration | Request timeout | `5s` |
| `tls` | object | TLS configuration | |
| `auth` | object | HTTP authentication | |
| `headers` | map | Additional HTTP headers | |
| `request_body` | object | Request body template | |
| `response` | object | Response parsing configuration | |
| `accounting` | object | Accounting event configuration | |

## TLS

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `insecure_skip_verify` | bool | Skip TLS certificate verification | `false` |
| `ca_cert_file` | string | Path to CA certificate file | `/etc/ssl/certs/ca.pem` |
| `cert_file` | string | Path to client certificate file | `/etc/ssl/client.pem` |
| `key_file` | string | Path to client private key file | `/etc/ssl/client-key.pem` |

## Auth

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `type` | string | Authentication type: `basic` or `bearer` | `bearer` |
| `username` | string | Username for basic auth | `admin` |
| `password` | string | Password for basic auth | |
| `token` | string | Token for bearer auth | |

## Request Body

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `template` | string | Go template for the request body; has access to subscriber context variables | |

## Response

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `allowed_condition` | object | Condition to determine if the subscriber is allowed | |
| `attribute_mappings` | array | Map JSON response fields to subscriber attributes | |

### Allowed Condition

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `jsonpath` | string | JSONPath expression to evaluate | `$.allowed` |
| `value` | string | Expected value for the condition to pass | `true` |

### Attribute Mapping

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `path` | string | JSONPath to extract the value from the response | `$.ip_address` |
| `attribute` | string | Subscriber attribute to set | `Framed-IP-Address` |

## Accounting

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `enabled` | bool | Enable accounting event notifications | `true` |
| `start` | object | Accounting start event configuration | |
| `update` | object | Accounting interim update event configuration | |
| `stop` | object | Accounting stop event configuration | |

### Accounting Event

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `endpoint` | string | URL for this accounting event | `https://auth.example.com/api/acct` |
| `method` | string | HTTP method | `POST` |
| `template` | string | Go template for the request body | |

## Example

```yaml
plugins:
  subscriber.auth.http:
    endpoint: https://auth.example.com/api/auth
    method: POST
    timeout: 5s
    auth:
      type: bearer
      token: my-secret-token
    response:
      allowed_condition:
        jsonpath: "$.allowed"
        value: "true"
      attribute_mappings:
        - path: "$.ip_address"
          attribute: Framed-IP-Address
        - path: "$.rate_limit"
          attribute: Rate-Limit
    accounting:
      enabled: true
      start:
        endpoint: https://auth.example.com/api/acct/start
        method: POST
      stop:
        endpoint: https://auth.example.com/api/acct/stop
        method: POST
```
