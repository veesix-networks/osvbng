# DHCPv6 LDRA Termination

osvbng's local DHCPv6 provider terminates inbound RFC 6221 Lightweight DHCPv6 Relay Agent (LDRA) traffic and emits matching Relay-Reply responses, so ISPs operating behind a wholesaler access network can allocate subscriber addresses and prefixes on-box while the wholesaler's access node continues to insert Interface-ID / Remote-Id via LDRA.

See also: [DHCP Relay and Proxy](DHCP_RELAY_PROXY.md) for the complementary upstream-forward role.

## When this applies

The LDRA termination path is engaged only when inbound DHCPv6 arrives as Relay-Forward (msg-type 12). Non-relayed DHCPv6 flows are unaffected. There is no per-subscriber or per-profile switch to enable LDRA termination: the ingress path inspects the message type and, if it is a Relay-Forward, unwraps and passes the relay context through to the provider.

A per-subscriber-group toggle (`dhcpv6.allow-relay-forward`, default `true`) lets operators running pure access networks explicitly reject inbound Relay-Forward as a defensive guard against misconfigured CPE.

## Flow

```
LDRA                              osvbng
  │ Solicit                         │
  │ ───► Relay-Forward(Solicit) ───►│
  │                                 │ unwrap → inner + RelayInfo
  │                                 │ allow-relay-forward check
  │                                 │ provider (local) allocates
  │                                 │ provider wraps Advertise
  │                                 │ in Relay-Reply, echoing
  │                                 │ Interface-ID verbatim
  │ ◄─── Relay-Reply(Advertise) ◄───│
  │ decap, deliver to client        │
```

## Wire format

The Relay-Reply echoes the inbound Interface-ID (RFC 3315 §20.2), copies the link-address and peer-address from the Relay-Forward, and preserves the hop-count so the LDRA can route the response to the correct downstream port.

Remote-Id (option 37) is not echoed. `RelayInfo.RemoteID` stores the post-enterprise-number bytes, so the original 4-byte enterprise prefix is not reconstructible, and RFC 3315 §20.2 does not mandate Remote-Id echo. The LDRA owns its own Remote-Id state locally.

## Per-subscriber-group opt-out

```yaml
subscriber-groups:
  groups:
    access-only-isp:
      access-type: ipoe
      vlans:
        - svlan: 100
          interface: loop100
      dhcpv6:
        allow-relay-forward: false
```

With `allow-relay-forward: false`, inbound Relay-Forward on this group is dropped after successful decode (not before), preserving the observability distinction between malformed-packet parse errors and policy rejects. A log line is emitted at INFO level with the S-VLAN and source MAC.

## Session state

LDRA termination stores no per-session relay state. Pending DHCPv6 packets (stashed while a Solicit awaits AAA approval) hold the raw outer frame so the async replay path re-runs the unwrap and reconstructs the relay context from the bytes, mirroring the DHCPv4 giaddr-carrying-bytes pattern. This keeps IPoE session state identical for LDRA-fronted and directly-attached subscribers.

## Not supported

- **Multi-hop Relay-Forward chains** (access LDRA plus aggregation relay). The decoder returns only the innermost relay context today; deeper chains flatten to single-hop on the reply.
- **Server-initiated Reconfigure** (RFC 3315 msg-type 10) and **Information-Request** (msg-type 11). Neither path is dispatched by the local provider today.
- **Leasequery** (msg-type 14 / 15).
