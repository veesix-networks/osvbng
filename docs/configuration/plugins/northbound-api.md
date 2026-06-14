# northbound.api

REST API for management and operational queries. Serves on a TCP
socket and a Unix domain socket; both expose the same handlers and
OpenAPI spec.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `enabled` | bool | Enable the plugin. Default `false`. | `true` |
| `listen_address` | string | TCP listen address. Default `:8080`. | `:8080` |
| `vrf` | string | Linux VRF master to bind the TCP listener to. | `mgmt-vrf` |
| `tls` | [TLS](#tls) | TLS settings for the TCP listener (HTTPS). | |
| `uds` | [UDS](#uds) | Unix-domain-socket listener settings. | |

`tls` and `vrf` apply to the TCP listener only.

## TLS

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `cert_file` | string | Path to the TLS certificate. | `/etc/osvbng/tls/api.crt` |
| `key_file` | string | Path to the TLS key. | `/etc/osvbng/tls/api.key` |

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
    listen_address: ":8080"
    uds:
      enabled: true
      path: /run/osvbng/api.sock
      mode: "0660"
      group: osvbng
```

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
