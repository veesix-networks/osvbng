# Handlers

Handlers are the mechanism that connects osvbng's configuration and show systems to the actual runtime behavior of components. They act as the bridge between user commands and system state.

## What are Handlers?

When a user runs a command like `set interfaces eth0 enabled true` or `show subscriber sessions`, a handler performs the actual work:

- **Config Handlers** - Validate and apply configuration changes to running components
- **Show Handlers** - Collect and display current system state

Handlers are registered by both core components and plugins, allowing the system to be extended without modifying the core CLI or configuration infrastructure.

## Handler Types

### Config Handlers

Config handlers manage the lifecycle of configuration changes:

**Validation Phase**
When a user runs `set interfaces eth0 enabled true`:
- Handler validates the new value (type checking, business logic)
- If invalid, user sees error immediately
- If valid, value written to candidate config

**Apply Phase**
When a user runs `commit`:
- Handlers apply changes to running components in dependency order
- Each handler updates its component's runtime state
- If any handler fails, all changes rollback automatically

**Rollback Phase**
If a commit fails partway through:
- Previously applied handlers undo their changes
- System restored to pre-commit state
- No partial or inconsistent configuration

**Example Flow:**
```
User: set interfaces eth0 enabled true
  → InterfaceHandler.Validate() ✓
  → Value written to candidate config

User: commit
  → InterfaceHandler.Apply() → Enables eth0 in VPP
  → BGPHandler.Apply() → Reconfigures BGP neighbor
  → Success! Changes committed
```

### Show Handlers

Show handlers query component state and format it for display:

**Collection Phase**
When a user runs `show subscriber sessions`:
- Handler queries the subscriber component
- Collects current session information
- Returns structured data

**Display Phase**
- System formats data based on output format (text, JSON, YAML)
- User sees current runtime state

**Example Flow:**
```
User: show subscriber sessions
  → SessionHandler.Collect() → Queries subscriber component
  → Returns list of active sessions
  → CLI formats and displays
```

## Handler Architecture

### Path-Based Routing

Handlers register for specific configuration or show paths:

**Config Path Examples:**
- `interfaces.*` - Handles all interface configuration
- `protocols.bgp.asn` - Handles BGP ASN changes
- `plugins.example.hello.message` - Handles plugin-specific config

**Show Path Examples:**
- `subscriber.sessions` - Shows subscriber sessions
- `interfaces.statistics` - Shows interface statistics
- `example.hello.status` - Shows plugin status

When a user runs a command, the system:
1. Translates command to path
2. Looks up handler for that path
3. Calls the appropriate handler method

### Dependency Management

Config handlers can declare dependencies to ensure correct application order:

**Example:**
- Address handler depends on `interfaces.*`
- Ensures interfaces exist before addresses are configured
- System applies in dependency order automatically

This prevents race conditions and ordering issues during commit.

### Handler Registration

Handlers self-register via factory functions:

**Core Handlers:**
Defined in `pkg/conf/handlers/` and `pkg/show/handlers/`

**Plugin Handlers:**
Defined in plugin directories (e.g., `plugins/community/hello/conf/` and `plugins/community/hello/show/`)

All handlers are discovered and registered automatically at startup - no central registry to maintain.

## Config Handler Lifecycle

A configuration change flows through multiple handler methods:

```
┌──────────────┐
│ User runs    │
│ 'set' command│
└──────┬───────┘
       │
       ↓
┌──────────────┐
│ PathPattern  │  Which path does this handler manage?
│ matching     │
└──────┬───────┘
       │
       ↓
┌──────────────┐
│ Validate()   │  Is this value valid?
└──────┬───────┘
       │
   [Valid]
       ↓
┌──────────────┐
│ Candidate    │  Write to candidate config
│ update       │
└──────┬───────┘
       │
       ↓
┌──────────────┐
│ User runs    │
│ 'commit'     │
└──────┬───────┘
       │
       ↓
┌──────────────┐
│ Dependency   │  Sort handlers by dependencies
│ resolution   │
└──────┬───────┘
       │
       ↓
┌──────────────┐
│ Apply()      │  Apply change to running component
└──────┬───────┘
       │
   [Success] ────────┐
       │             │
   [Failure]         │
       │             │
       ↓             ↓
┌──────────────┐   ┌──────────────┐
│ Rollback()   │   │ Running      │
│ all changes  │   │ config       │
└──────────────┘   │ updated      │
                   └──────────────┘
```

### Key Methods

**PathPattern()**
Declares which configuration path this handler manages.

**Validate()**
Checks if a proposed change is valid before it's committed. Called immediately when user runs `set`.

**Apply()**
Makes the change active in the running component. Called during `commit`.

**Rollback()**
Undoes a previously applied change if commit fails. Ensures atomicity.

**Dependencies()**
Declares which other paths must be configured first. Ensures correct ordering.

**Callbacks()**
Optional pre/post hooks for coordinating with other systems (e.g., FRR reload).

## Show Handler Lifecycle

A show command flows through:

```
┌──────────────┐
│ User runs    │
│ 'show'       │
│ command      │
└──────┬───────┘
       │
       ↓
┌──────────────┐
│ PathPattern  │  Which path does this handler provide?
│ matching     │
└──────┬───────┘
       │
       ↓
┌──────────────┐
│ Collect()    │  Query component for current state
└──────┬───────┘
       │
       ↓
┌──────────────┐
│ Format       │  Convert to JSON/YAML/text
└──────┬───────┘
       │
       ↓
┌──────────────┐
│ Display      │  Show to user
│ to user      │
└──────────────┘
```

### Key Methods

**PathPattern()**
Declares which show path this handler provides.

**Collect()**
Queries components and returns structured data representing current state.

**Dependencies()**
Declares which other show handlers must run first (rarely used).

## Handler Integration with Components

Handlers connect the configuration system to component runtime state:

**Config Handler → Component:**
```
User: set example hello message "test"
  → MessageHandler.Apply()
    → Gets hello.Component instance
    → Calls component.SetMessage("test")
    → Component updates its runtime state
```

**Show Handler → Component:**
```
User: show example hello status
  → StatusHandler.Collect()
    → Gets hello.Component instance
    → Calls component.GetMessage()
    → Returns current state to user
```

This separation of concerns means:
- Components focus on business logic
- Handlers focus on configuration translation
- Configuration system stays generic

## FRR Callbacks

Some configuration changes require FRR (routing daemon) reload:

**BGP configuration changes:**
- Handler applies change to component
- Handler requests FRR reload via callback
- System generates FRR config from new candidate
- FRR validates config
- If valid, FRR reloads
- If invalid, entire commit rolls back

This ensures routing configuration stays consistent with interface and VRF configuration.

## Plugin Handlers

Plugins contribute their own handlers to extend the configuration and show systems:

**Plugin Config Handler:**
- Handles `plugins.example.hello.message` path
- Validates plugin-specific configuration
- Applies changes to plugin component

**Plugin Show Handler:**
- Handles `example.hello.status` path
- Displays plugin runtime state

Plugins use the same handler infrastructure as core components - no special treatment needed.

## Implementation Details

For detailed implementation guidance, see:
- Example config handler: `plugins/community/hello/conf/message.go`
- Example show handler: `plugins/community/hello/show/status.go`
- Core handlers: `pkg/conf/handlers/` and `pkg/show/handlers/`

## Related Documentation

- [Plugin Development](plugins/PLUGINS.md) - How to build plugins with handlers
- [Component System](COMPONENTS.md) - Understanding components
- [Configuration System](CONFIGURATION.md) - How config management works
