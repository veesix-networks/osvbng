# Component System

The component system is osvbng's core architectural pattern for building modular, lifecycle-managed functionality. Components are self-contained units that encapsulate specific features or capabilities within the BNG.

## What is a Component?

A component is an independent module with a defined lifecycle that:
- Starts and stops cleanly
- Manages its own resources and goroutines
- Can communicate with other components via events
- Registers itself automatically at compile time
- Can be enabled/disabled via configuration

Think of components as microservices within a single process - each has its own responsibility and lifecycle, but they share common infrastructure.

## Component Types

### Core Components

Core components provide essential BNG functionality and live in `internal/`:

- **subscriber** - Manages subscriber sessions and state
- **aaa** - Authentication, authorization, and accounting
- **routing** - BGP and routing protocol integration
- **dataplane** - VPP dataplane control and management
- **monitor** - Metrics collection and system monitoring
- **gateway** - gRPC API for external access
- **ipoe** - IPoE subscriber protocol handling
- **arp** - ARP protocol handling

Core components are always compiled in but can be conditionally enabled via configuration.

### Plugin Components

Plugin components extend osvbng with optional or vendor-specific functionality:

- **Community plugins** (`plugins/community/`) - User-contributed features and extensions
- **Exporters** (`plugins/exporter/`) - Metrics exporters like Prometheus, SNMP
- **DHCPv4/DHCPv6 components** (`plugins/dhcp4/`, `plugins/dhcp6/`) - Protocol handlers for DHCPv4 and DHCPv6

Plugin components follow the same lifecycle and interfaces as core components but are maintained separately.

### Providers

Providers are different to components in that they don't implement a brand new feature set, but swap the behavior within an existing component:

- **Auth providers** (`plugins/auth/`) - Authentication backends (RADIUS, local, LDAP, HTTP, etc..)
- **Cache providers** (`plugins/cache/`) - Cache implementations (Redis, memcached, in-memory)
- **DHCP providers** (`plugins/dhcp4/local`, `plugins/dhcp4/relay`) - DHCP implementation types (local server, relay, proxy)

Providers have no lifecycle and are used by components that need pluggable behavior (e.g., AAA component can use different auth providers, DHCPv4 component can use local vs relay provider).

## Component Lifecycle

All components follow a consistent lifecycle:

```
Registration → Creation → Start → Running → Stop
```

### 1. Registration (Compile Time)

Components register their factory functions when the package is imported:

```go
func init() {
    component.Register("subscriber", NewComponent)
}
```

This happens automatically - no manual registration needed.

### 2. Creation (Startup)

During osvbng startup, registered component factories are called. Each factory:
- Checks if the component should be created based on configuration
- Returns the component instance, or nil if disabled
- Returns an error if creation fails critically

### 3. Start (Initialization)

Once all components are created, they're started in order:
- Initialize resources (connections, channels, workers)
- Start background goroutines
- Register with other systems
- Become ready to handle work

### 4. Running (Operational)

Component performs its function:
- Processing events from other components
- Handling protocol packets
- Managing subscriber state
- Responding to configuration changes

### 5. Stop (Shutdown)

During osvbng shutdown, components stop cleanly:
- Stop accepting new work
- Cancel background goroutines
- Release resources
- Persist state if needed

## Component Communication

Components communicate through shared infrastructure rather than direct dependencies:

### Event Bus

Publish/subscribe system for inter-component events.

**Example events:**
- `session.created` - Subscriber component notifies of new sessions
- `session.deleted` - Session removed
- `auth.success` - AAA completed successfully

This decouples components - they don't need direct references to each other.

### Shared Cache

Common cache for temporary state that multiple components need to access.

### VPP Southbound

Shared interface to the VPP dataplane for forwarding operations.

### Packet Channels

Components receive parsed packets from the dataplane:
- DHCP channel for DHCP components
- ARP channel for ARP handler
- PPP channel for PPPoE components

## Component vs Provider

We've already discussed how the component is different to the provider but here is a recap with some examples. This distinction is extremely important in the osvbng architecture so here is a summary with examples:

### Component
A component has its own lifecycle and runs independently:
- **Has Start/Stop methods** - Manages its own resources and lifecycle
- **Registers via** `component.Register()`
- **Examples:**
  - Prometheus exporter (runs HTTP server, collects metrics)
  - Subscriber manager (manages sessions, handles events)
  - Monitoring daemon (collects stats, publishes data)
  - Hello plugin (example feature with its own behavior)

### Provider
A provider swaps out implementation of an existing interface within a component:
- **No lifecycle** - Just implements an interface used by a component
- **Registers via provider-specific function** (e.g., `auth.RegisterProvider()`, `cache.RegisterProvider()`)
- **Examples:**
  - RADIUS auth provider (implements authentication interface for AAA component)
  - Redis cache provider (implements cache interface for cache component)
  - Local auth provider (implements authentication interface with local file/database)

**Key Difference:**
- **Component** = Standalone feature that runs on its own
- **Provider** = Pluggable implementation that changes how an existing component behaves

A provider modifies behavior at specific points within a component, while a component adds entirely new functionality to osvbng.

## Related Documentation

- [Plugin Development](PLUGINS.md) - How to build plugin components
- [Configuration System](CONFIGURATION.md) - How configuration works
- [Handlers](HANDLERS.md) - Config and show handlers
