# Event Bus

osvbng components communicate through a publish/subscribe event bus. This is the primary mechanism for inter-component communication - components don't hold direct references to each other.

## Using the Event Bus

The event bus is available via `component.Dependencies.EventBus` for plugin components, or injected directly for core components.

### Publishing

```go
c.eventBus.Publish(events.TopicSubscriberTerminate, events.Event{
    Source:    c.Name(),
    Timestamp: time.Now(),
    Data: &events.SubscriberTerminateEvent{
        AcctSessionID: "abc123",
        Reason:        "admin-clear",
    },
})
```

Publishing is fire-and-forget. The bus has a bounded internal channel (capacity 10,000). If the channel is full, events are dropped with a log warning and counter increment.

### Subscribing

```go
sub := c.eventBus.Subscribe(events.TopicSubscriberMutation, func(ev events.Event) {
    data, ok := ev.Data.(*events.SubscriberMutationEvent)
    if !ok {
        return
    }
    // handle the event
})

// Later, during Stop:
sub.Unsubscribe()
```

Each subscriber handler runs in its own goroutine. There is no ordering guarantee between handlers for the same topic.

## Topics

<span class="event-topic">topic</span> = event bus topic constant &nbsp;&nbsp; <span class="event-type">EventType</span> = Go struct carried in `ev.Data`

### Session Lifecycle

<span class="event-topic">session:lifecycle</span> <span class="event-type">SessionLifecycleEvent</span>

Session state changes (active, released). Published by access components (PPPoE, IPoE) when a session transitions state. Consumed by:

- Subscriber component - persistence, QoS application
- AAA component - accounting Start/Interim/Stop
- HA sync - session replication to standby

<span class="event-topic">session:programmed</span>

Session has been programmed into the VPP dataplane.

### AAA

<span class="event-topic">aaa:request</span> <span class="event-type">AAARequestEvent</span>

Authentication request from an access component. Consumed by the AAA component which dispatches to the configured auth provider.

<span class="event-topic">aaa:response</span> <span class="event-type">AAAResponseEvent</span>

Generic AAA response.

<span class="event-topic">aaa:response:ipoe</span> <span class="event-type">AAAResponseEvent</span>

AAA response routed to the IPoE component.

<span class="event-topic">aaa:response:pppoe</span> <span class="event-type">AAAResponseEvent</span>

AAA response routed to the PPPoE component.

### Subscriber Mutation

<span class="event-topic">subscriber:mutation</span> <span class="event-type">SubscriberMutationEvent</span>

Request to change attributes on a live session. Published by the subscriber component (via oper handler) or by plugins (e.g., RADIUS CoA). Both PPPoE and IPoE components receive every event and resolve the target via in-memory indexes. Only the component that owns the matching session applies the delta.

<span class="event-topic">subscriber:mutation:result</span> <span class="event-type">SubscriberMutationResultEvent</span>

Result of a mutation request. Published by the access component that handled it. Consumed by:

- Subscriber component - cache persistence, waiter resolution
- HA sync - session replication

### Subscriber Terminate

<span class="event-topic">subscriber:terminate</span> <span class="event-type">SubscriberTerminateEvent</span>

Request to tear down a live session. Published by the subscriber component (via `clear` oper handler) or by plugins (e.g., RADIUS Disconnect-Message). Both PPPoE and IPoE components receive every event. The component that owns the matching session deprovisions it from VPP, releases IP allocations, and publishes a `SessionLifecycleEvent` with state=released. Fire-and-forget - no result event is published.

### Infrastructure

<span class="event-topic">egress</span> <span class="event-type">EgressEvent</span>

Egress packet for transmission.

<span class="event-topic">ha:state_change</span> <span class="event-type">HAStateChangeEvent</span>

HA SRG state transition (active/standby). Consumed by access components and CGNAT to enable/disable southbound programming.

<span class="event-topic">interface:state</span> <span class="event-type">InterfaceStateEvent</span>

Interface admin/link state change from VPP.

<span class="event-topic">cgnat:mapping</span> <span class="event-type">CGNATMappingEvent</span>

CGNAT port-block mapping created or deleted. Consumed by HA sync for CGNAT state replication.

## Event Types

### SubscriberMutationEvent

Published on `TopicSubscriberMutation`. Identifies a target session by one of the target fields and carries the attribute delta to apply.

```go
type SubscriberMutationEvent struct {
    RequestID      string            // Unique request ID for result correlation
    SessionID      string            // Target by internal session ID
    AcctSessionID  string            // Target by Acct-Session-Id
    Username       string            // Target by username
    FramedIPv4     string            // Target by IPv4 address
    FramedIPv6     string            // Target by IPv6 address
    AttributeDelta map[string]string // Attributes to set on the session
}
```

Exactly one target field should be non-empty. Both PPPoE and IPoE components receive every event - only the component that owns the matching session handles it.

### SubscriberMutationResultEvent

Published on `TopicSubscriberMutationResult`. Reports the outcome of a mutation.

```go
type SubscriberMutationResultEvent struct {
    RequestID  string                    // Correlates to the request
    SessionID  string                    // Internal session ID
    Ok         bool                      // Whether the mutation succeeded
    Error      string                    // Error message if failed
    ErrorCause int                       // RFC 5176 Error-Cause code (0 on success)
    Session    models.SubscriberSession  // Updated session snapshot (non-nil on success)
}
```

### SubscriberTerminateEvent

Published on `TopicSubscriberTerminate`. Requests session teardown by one of the target fields.

```go
type SubscriberTerminateEvent struct {
    SessionID     string // Target by internal session ID
    AcctSessionID string // Target by Acct-Session-Id
    Username      string // Target by username
    FramedIPv4    string // Target by IPv4 address
    FramedIPv6    string // Target by IPv6 address
    Reason        string // Human-readable reason (for logging only)
}
```

Fire-and-forget - no result event. The access component handles teardown and publishes a `SessionLifecycleEvent` with state=released through the normal lifecycle path.

### SessionLifecycleEvent

Published on `TopicSessionLifecycle`. Core session state transition event.

```go
type SessionLifecycleEvent struct {
    AccessType models.AccessType  // "ipoe" or "pppoe"
    Protocol   models.Protocol    // "dhcpv4", "pppoe_session", etc.
    SessionID  string
    State      models.SessionState // "active", "released", etc.
    Session    any                 // *models.IPoESession or *models.PPPSession
}
```

### HAStateChangeEvent

Published on `TopicHAStateChange`. Signals an SRG state transition.

```go
type HAStateChangeEvent struct {
    SRGName  string // SRG name
    OldState string // Previous state
    NewState string // New state
}
```

## For Plugin Developers

Plugin components receive `component.Dependencies` which includes `EventBus`. To subscribe to events:

1. Subscribe in your `Start()` method
2. Unsubscribe in your `Stop()` method
3. Type-assert `ev.Data` to the expected event type

### Safe Topics for Plugins

These topics are designed for plugin consumption and publication:

| Topic | Sub | Pub | Use Case |
|-------|-----|-----|----------|
| `TopicSessionLifecycle` | Yes | No | Session create/release |
| `TopicSubscriberMutation` | Yes | Yes | Attribute changes |
| `TopicSubscriberMutationResult` | Yes | No | Mutation outcomes |
| `TopicSubscriberTerminate` | No | Yes | Session teardown |
| `TopicHAStateChange` | Yes | No | HA failover |
| `TopicInterfaceState` | Yes | No | Link state changes |
| `TopicCGNATMapping` | Yes | No | CGNAT mapping events |

Common plugin use cases:

- **CDR / billing** - subscribe to `TopicSessionLifecycle` for session start/stop
- **External IPAM** - subscribe to `TopicSessionLifecycle` to sync IP allocations
- **Lawful intercept** - subscribe to `TopicSessionLifecycle` to activate/deactivate taps
- **CoA / policy push** - publish to `TopicSubscriberMutation` with attribute changes
- **Admin disconnect** - publish to `TopicSubscriberTerminate` with session target
- **Standby awareness** - subscribe to `TopicHAStateChange` to disable features on standby

### Internal Topics - Do Not Use in Plugins

!!! warning "These topics are internal control-plane contracts"
    Publishing to or subscribing to these topics from a plugin will interfere with core component logic and may cause authentication bypass, session state corruption, or packet processing failures.

**`TopicAAARequest`** - Only the AAA component consumes this to dispatch to the configured auth provider. A plugin subscribing would race the authentication flow.

**`TopicAAAResponse` / `TopicAAAResponseIPoE` / `TopicAAAResponsePPPoE`** - Only the access components consume these. A plugin publishing fake auth responses would bypass authentication.

**`TopicSessionProgrammed`** - Internal signal between the dataplane and access components for VPP programming coordination.

**`TopicEgress`** - Internal packet transmission path. Publishing here injects packets into the dataplane.

### Example

See the [RADIUS CoA configuration](../configuration/plugins/auth-radius.md#coa--disconnect-message-rfc-5176) for a working example of a plugin component that uses mutation and terminate events.
