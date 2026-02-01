# State Collectors

## Overview

State collectors gather operational metrics from components and plugins, storing them in cache for consumption by exporters. This decouples data collection from data presentation, allowing multiple export targets (Prometheus, SNMP agents, Telegraf+Grafana, etc.) to consume the same cached metrics without repeatedly querying components.

Note: Northbound systems like the CLI or gRPC API call show handlers directly for real-time data. The cache is primarily for exporters that poll/scrape periodically.

## Purpose

Collectors solve the problem of efficient metric export to external monitoring systems. Instead of exporters directly querying components (which would query components N times for N exporters), collectors gather data periodically (every N seconds) and store snapshots in cache. Exporters then read from cache and transform data into their target format. This means components are queried once per interval regardless of how many exporters are consuming the data.

## How It Works

Collectors run on a configured interval (default: 5 seconds). Each collector:

1. Wraps a show handler and calls it periodically
2. Serializes the result to JSON
3. Stores it in cache with a TTL (default: 30 seconds)

Exporters read from cache and transform data as needed for their target system.

## Implementation

Collectors are registered directly in show handler files using `state.RegisterMetric()`. No separate collector files needed - the collector automatically wraps the show handler.

Example from `pkg/handlers/show/protocols/bgp/statistics.go`:

```go
func init() {
    show.RegisterFactory(NewBGPStatisticsHandler)

    state.RegisterMetric(statepaths.ProtocolsBGPStatistics, paths.ProtocolsBGPStatistics)
    state.RegisterMetric(statepaths.ProtocolsBGPIPv6Statistics, paths.ProtocolsBGPIPv6Statistics)
}
```

### Wildcard Collectors

Collectors support wildcard paths to automatically collect metrics for all instances of a dynamic resource. When a collector is registered with a wildcard path like `protocols.bgp.neighbors.<*:ip>`, the system:

1. Examines the running configuration
2. Resolves all instances matching the wildcard pattern
3. Calls the show handler once for each instance
4. Caches each result separately

Example from `pkg/handlers/show/protocols/bgp/neighbors.go`:

```go
func init() {
    show.RegisterFactory(NewBGPNeighborsHandler)

    // This automatically collects metrics for ALL configured BGP neighbors
    state.RegisterMetric(statepaths.ProtocolsBGPNeighbors, paths.ProtocolsBGPNeighbors)
}
```

If you have neighbors `10.4.20.254` and `10.5.30.1` configured, the collector will:
- Call handler for `protocols.bgp.neighbors.10.4.20.254`
- Call handler for `protocols.bgp.neighbors.10.5.30.1`
- Cache both results separately

This enables per-instance metrics export (e.g., separate Prometheus metrics for each BGP neighbor).

## For Plugin Developers

To enable periodic caching of your plugin's data for exporters, register a collector using `state.RegisterMetric(cachePath, handlerPath)` in your show handler's init() function.

See `PLUGINS.md` for complete implementation details.

## Configuration

All registered collectors run by default. To disable specific collectors:

```yaml
monitoring:
  collect_interval: 5s
  disabled_collectors:
    - memory
```

If `disabled_collectors` is empty or omitted, all registered collectors run.