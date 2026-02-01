# Configuration System

The osvbng configuration system provides transactional, versioned configuration management similar to traditional network devices, however this is typically automatically-rendered via the internals, we would like to approach osvbng with an API first approach and provide multiple northbound systems, however a CLI will always warm the network engineers heart. It follows a candidate configuration model with validation, atomic commits, and automatic rollback on errors.

## Core Concepts

### Configuration Types

osvbng maintains three types of configuration similar to NETCONF, as you read on you will find a lot of influences from NETCONF are natively integrated into osvbng's architecture:

**Running Config**
- Currently active configuration
- What the system is actually using
- Only changes when you commit

**Candidate Config**
- Configuration being edited
- Exists per-session
- Discarded if not committed

**Startup Config**
- Configuration loaded on boot
- Persisted to `/etc/osvbng/startup-config.yaml`
- Automatically updated when changes are committed

This separation ensures you can make changes, validate them, and commit atomically - or discard them without affecting the running system.

### Configuration Sessions

Each user/connection gets their own isolated configuration session with a private candidate config. Multiple users can edit simultaneously without interfering with each other.

**Session Lifecycle:**
```
Create Session → Make Changes → Commit/Discard → Close Session
```

Changes made in your session are invisible to others until committed.

## Configuration Workflow

### CLI Example

```bash
osvbngcli> configure
[config]> set interfaces eth0 address ipv4 192.168.1.1/24
[config]> set interfaces eth0 enabled true
[config]> set protocols bgp asn 65000
[config]> commit
Configuration committed successfully
[config]> exit
```

**What happens:**
1. `configure` - Creates candidate session (copy of running config)
2. `set` commands - Modify candidate config (validated immediately)
3. `commit` - Apply all changes transactionally to running config
4. `exit` - Close session and discard candidate

### Transaction Model

Configuration changes are **atomic transactions**:

- Either all changes apply successfully, or none do
- Failed commit automatically rolls back all changes
- Running config never left in inconsistent state
- Validation happens before any changes take effect

**Example:**
```
set interfaces eth0 enabled true          ✓ Valid
set protocols bgp asn 65000               ✓ Valid
set protocols bgp router-id 10.0.0.1      ✓ Valid
commit → All 3 changes applied atomically
```

```
set interfaces eth0 enabled true          ✓ Valid
set protocols bgp asn invalid             ✗ Invalid
commit → FAILS, no changes applied (even the valid ones)
```

## Configuration Structure

The configuration is organized into logical sections for different aspects of the BNG (interfaces, protocols, AAA, VRFs, etc.).

**Plugins:**
Plugins define their own configuration sections under the `plugins:` key:

```yaml
plugins:
  example.hello:
    enabled: true
    message: "Custom greeting"
```

Each plugin registers its own typed configuration structure and can access it at runtime.

Detailed configuration schema documentation is auto-generated from the code.

## Configuration Handlers

Handlers are the bridge between configuration and runtime behavior:

**Config Handlers**
- Validate configuration changes
- Apply changes to running components
- Rollback on failure

**Show Handlers**
- Display current system state
- Query component runtime information

When you `set interfaces eth0 enabled true`, a config handler:
1. Validates the value
2. Writes it to candidate config
3. On commit, applies it to the VPP dataplane
4. On failure, rolls it back

See [HANDLERS.md](HANDLERS.md) for implementation details.

## Configuration Versioning

Every commit creates a versioned snapshot stored in `/etc/osvbng/versions/`:

```yaml
version: 42
timestamp: 2026-01-03T10:30:00Z
changes:
  - type: add
    path: interfaces.eth0.enabled
    value: true
  - type: modify
    path: protocols.bgp.asn
    value: 65000
```

**Benefits:**
- Audit trail of all configuration changes
- Ability to rollback to previous versions (planned)
- Disaster recovery
- Change tracking

Versioning can be disabled for testing/development environments.

## Path-Based Configuration

Configuration uses space-separated paths to address specific values:

```
set interfaces eth0 enabled true
set protocols bgp asn 65000
set example hello message "test"
```

The CLI translates these commands into internal configuration paths that map to the YAML structure. This provides a consistent interface regardless of whether you're using CLI, gRPC API, or configuration files.

## FRR Integration

Some configuration changes affect the FRR routing daemon and require coordination to maintain consistency.

**Routing configuration changes:**
When you commit changes that affect routing (BGP, OSPF, IS-IS, static routes, VRFs):
1. All config handlers apply their changes
2. FRR configuration is generated from the new config
3. FRR validates the configuration
4. If valid, osvbng diffs the new and running FRR configuration and applies only what's needed
5. If invalid, the entire commit rolls back (including non-FRR changes)

This ensures routing and interface configurations stay synchronized atomically.

## Plugin Configuration

Plugins define their own configuration structures and register them with the configuration system. Plugin configurations are stored under the `plugins:` section of the unified config file:

```yaml
plugins:
  example.hello:
    enabled: true
    message: "Hello world"
```

Each plugin registers a typed configuration structure, allowing type-safe access while maintaining a single unified configuration file. Plugin configurations follow the same transactional model as core configuration - changes are validated, committed atomically, and versioned.

See [Plugin Development](PLUGINS.md) for implementation details.

## Related Documentation

- [Handlers](HANDLERS.md) - Implementing config and show handlers
- [Plugin Development](PLUGINS.md) - Building plugins with configuration
- [Component System](COMPONENTS.md) - Component architecture
