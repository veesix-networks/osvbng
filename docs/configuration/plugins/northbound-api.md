# northbound.api

REST API for management and operational queries. Serves on one or
more TCP sockets (each can bind a different Linux VRF and use
different TLS material) plus a Unix domain socket. All sockets expose
the same handlers and OpenAPI spec.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `enabled` | bool | Enable the plugin. Default `false`. | `true` |
| `listeners` | [][Listener](#listener) | TCP listeners. Empty means no TCP exposure. | |
| `uds` | [UDS](#uds) | Unix-domain-socket listener settings. | |

The defaults block (the auto-generated config emitted by `osvbngd
config`) ships `listeners: []` and `uds.enabled: true`, so a fresh
deployment is reachable only via the Unix socket until the operator
declares a TCP listener.

## Listener

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `address` | string | TCP listen address. | `:8080` |
| `vrf` | string | Linux VRF master to bind this listener to. | `mgmt-vrf` |
| `tls` | [TLS](#tls) | TLS settings for this listener (HTTPS). | |

Each listener runs in the same osvbngd process and shares the same
mux, OpenAPI spec, and configuration state. Per-listener bind
failures (port in use, unknown VRF) are logged and skipped; other
listeners stay up.

## TLS

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `cert_file` | string | Path to the TLS certificate. | `/etc/osvbng/tls/api.crt` |
| `key_file` | string | Path to the TLS key. | `/etc/osvbng/tls/api.key` |
| `ca_cert_file` | string | CA bundle for client-cert verification (mTLS). | `/etc/osvbng/tls/ca.crt` |
| `client_auth` | string | Client-cert policy: `request`, `require`, or unset. | `require` |
| `min_version` | string | Minimum TLS version: `1.2` or `1.3`. | `1.3` |

## UDS

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `enabled` | bool | Enable the Unix-socket listener. Default `true` in the generated config. | `true` |
| `path` | string | Socket path. Default `/run/osvbng/api.sock`. | `/run/osvbng/api.sock` |
| `mode` | string | Octal permissions applied after bind. Default `"0660"`. | `"0660"` |
| `group` | string | Group ownership applied after bind. Default `osvbng`. Unknown group is logged and skipped; socket stays as `root:root`. | `osvbng` |

## Example

```yaml
plugins:
  northbound.api:
    enabled: true
    listeners:
      - address: ":8080"
        vrf: mgmt-vrf
        tls:
          cert_file: /etc/osvbng/tls/api-mgmt.crt
          key_file: /etc/osvbng/tls/api-mgmt.key
      - address: ":8443"
        tls:
          cert_file: /etc/osvbng/tls/api.crt
          key_file: /etc/osvbng/tls/api.key
    uds:
      enabled: true
      path: /run/osvbng/api.sock
      mode: "0660"
      group: osvbng
```

## Deprecated top-level fields

The legacy `listen_address`, `vrf`, and `tls` fields at the top level
of `northbound.api` (single-listener form) are still parsed for
backwards compatibility and produce a deprecation warning on startup.
A future release will remove them. Migrate by moving the values into
a single entry under `listeners:`.

Setting both the legacy fields and `listeners:` in the same config
is rejected at startup; pick one.

## Reaching the API

From any shell on the BNG:

```
osvbngcli
```

With no arguments, `osvbngcli` connects via the Unix socket if it
exists, otherwise falls back to `http://localhost:8080`.

To force a specific server:

```
osvbngcli --server unix:///run/osvbng/api.sock
osvbngcli --server http://localhost:8080
osvbngcli --server https://bng-mgmt.example.net
```

curl works against either listener:

```
curl --unix-socket /run/osvbng/api.sock http://unix/api/show/interfaces
curl http://localhost:8080/api/show/interfaces
```
