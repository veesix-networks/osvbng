# Monitoring

There is nothing to configure here. Metrics collection has no operator knobs.

Show handlers register the metrics they can produce with the telemetry registry at startup. The registry polls every registered handler on a fixed 10 second cadence and caches the result. Handlers whose component is disabled return nothing and are skipped, so metrics appear only for the features actually in use.

Exposing those metrics is a plugin's job. Enable [`exporter.prometheus`](plugins/exporter-prometheus.md) to serve them for scraping:

```yaml
plugins:
  exporter.prometheus:
    enabled: true
    listen_address: ":9090"
```

The `monitoring` block in `osvbng.yaml` is accepted by the config loader for backwards compatibility, but neither `collect_interval` nor `disabled_collectors` is read by anything. Setting them has no effect.
