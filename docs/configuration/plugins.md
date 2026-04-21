# Plugins

Plugin-specific configuration. Each key under `plugins` is a plugin namespace with its own configuration.

## Auth Providers

- [subscriber.auth.local](plugins/auth-local.md) - Local subscriber authentication
- [subscriber.auth.http](plugins/auth-http.md) - HTTP-based subscriber authentication
- [subscriber.auth.radius](plugins/auth-radius.md) - RADIUS authentication and accounting

## Exporters

- [exporter.prometheus](plugins/exporter-prometheus.md) - Prometheus metrics exporter
- [exporter.cgnat.http](plugins/exporter-cgnat-http.md) - HTTP exporter for CGNAT port-block allocate/release events (metadata retention / LI correlation)

## Northbound

- [northbound.api](plugins/northbound-api.md) - REST API for management

## Miscellaneous

- [example.hello](plugins/example-hello.md) - Community example plugin
