# L2GW (Layer 2 Wholesale Gateway)

The layer 2 wholesale gateway cross-connects subscriber circuits between
access networks and retail ISP handoff ports without terminating DHCP or L3.
osvbng acts as the wholesale aggregator: the first frame on a wholesale
circuit triggers AAA, the auth response selects a named **handoff group**
(plus optional explicit egress VLANs), and osvbng installs a bidirectional
L2 cross-connect with S/C-VLAN rewrite. From then on every frame (ARP, ND,
DHCP renewals, IGMP) is switched in the dataplane, and the retail ISP's BNG
terminates the subscriber. It is the IPoE analogue of LAC/LNS wholesale.

Two operating modes share one mechanism:

- **Static maps.** An entire access S-VLAN (all C-VLANs) is cross-connected
  to an ISP by configuration. No trigger, no RADIUS, no per-subscriber state.
- **Dynamic circuits.** Per-(S,C) circuits authorized by AAA, with egress
  VLANs allocated by osvbng or returned by RADIUS.

Dynamic circuits are created by one of two per-range trigger modes:

- **`trigger: dhcp`** (default). Only a DHCPv4 DISCOVER/REQUEST or DHCPv6
  SOLICIT/REQUEST on the circuit creates it. Right when every subscriber
  on the range speaks DHCP.
- **`trigger: packet`.** The first frame of **any** protocol creates the
  circuit. This removes the artificial requirement that the subscriber
  speak DHCP: PPPoE retail behind the wholesale line, static-IP business
  services, and IPv6-only CPE all work with the same mechanism.

## How the exit interface is chosen

The exit (handoff) interface is never named directly by RADIUS or by the
subscriber group. Both sides only ever reference a **handoff group label**,
and the label resolves to an interface in `l2gw.handoff-groups`. This keeps
the OSS/RADIUS integration stable when the physical wiring changes: re-point
the label at a new interface and every circuit follows.

Resolution order for a dynamic circuit:

1. `l2gw.handoff-group` attribute in the Access-Accept, if present.
2. Otherwise the subscriber group's `l2gw.handoff-group` default.
3. If neither exists, the circuit is rejected and logged.

The chosen group's `interface` field is the exit interface. The egress VLANs
on that interface come from the `l2gw.svlan` / `l2gw.cvlan` attributes when
RADIUS supplies them, and from the group's `svlan` / `svlan-range` /
`cvlan-range` allocator when it does not.

Static maps name their handoff group in configuration, so the exit interface
is fixed at config time.

The access side is typically not one network: each wholesale access
operator (altnet, muni fiber, open-access network) lands on its own NNI
port with its own VLAN plan, declared as its own subscriber group. l2gw
ranges are exempt from the single-access-interface constraint that
applies to locally terminated IPoE/PPPoE, so one osvbng can aggregate
any number of access operator NNIs toward any number of ISP handoffs.

## `l2gw.handoff-groups`

Bond/LACP interfaces are supported as handoff or access ports.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `interface` | string | Exit port toward the retail ISP (physical or bond). | `bond1` |
| `vlan-tpid` | string | Outer TPID emitted toward the handoff: `dot1ad` (default) or `dot1q`. | `dot1ad` |
| `svlan` | uint16 | Pin every circuit of this group to one outer VLAN (VLAN-per-ISP model). Mutually exclusive with `svlan-range`. | `200` |
| `svlan-range` | string | Outer VLAN allocator range for dynamic circuits. | `"200-299"` |
| `cvlan-range` | string | Inner VLAN allocator range for dynamic circuits. | `"1-4000"` |

## `l2gw.static-maps`

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `access-interface` | string | Access port the S-VLANs arrive on. | `eth1` |
| `svlan` | string | Single VLAN or range. | `"10-99"` |
| `handoff-group` | string | Target handoff group. | `isp-green` |
| `handoff-svlan` | uint16 | Translate the outer tag toward the ISP. Only valid for a single-SVLAN map, because a range sharing one egress S-VLAN would collide on the handoff side. | `210` |
| `transparent` | bool | Pass all tags untouched. Implied when neither `handoff-svlan` nor the group's fixed `svlan` is set. | `true` |

C-VLANs always pass through untouched on static maps (wildcard circuits).

## Subscriber group binding (dynamic mode)

Dynamic circuits are armed per VLAN range with the `l2gw` access type, which
is mutually exclusive with all other access types on that range:

```yaml
subscriber-groups:
  groups:
    wholesale-access-a:
      vlans:
        - svlan: "100-199"
          cvlan: any
          parent-interface: eth1
          access-types: [l2gw]
          trigger: packet         # any-protocol first-frame trigger (default: dhcp)
      l2gw:
        handoff-group: isp-blue   # default when AAA returns no label
        idle-timeout: 3600        # tear down after 1h without traffic (0/omitted: never)
      aaa-policy: line-policy     # e.g. format "$subscriber-group$.$svlan$.$cvlan$"
```

## Line identity and usernames

The identity of a wholesale line is the subscriber group (the access
operator's NNI) plus the S/C VLAN tuple its provisioning assigned. When
the range has no `aaa-policy`, or the policy format cannot be resolved
for a trigger, the username defaults to exactly that:
`<subscriber-group>.<svlan>.<cvlan>` (e.g. `wholesale-access-a.150.42`).
The trigger frame's source MAC is recorded on the circuit for show and
accounting, but it is never the identity: a CPE swap must not change the
subscriber.

Policies remain fully user-definable. The documented convention is:

```yaml
aaa:
  policy:
    - name: line-policy
      format: "$subscriber-group$.$svlan$.$cvlan$"
      password: wholesale
```

`$subscriber-group$` expands to the subscriber group name, so one policy
serves every access operator without a literal per-group prefix.

## Packet-triggered circuits (`trigger: packet`)

With `trigger: packet` on the range, the dataplane punts the first frame
of any ethertype on an unknown (armed, no circuit) S/C tuple; the
control plane authorizes on the tuple alone and installs the circuit.
Details that differ from DHCP mode:

- **No DHCP options in the AAA request.** The frame is not parsed beyond
  ethernet and VLAN tags, so option-82 `circuit_id`/`remote_id` (DHCPv4)
  and option 18/37 (DHCPv6 LDRA) are not available. Identity is the
  group-qualified VLAN tuple.
- **No trigger replay.** The held frame is dropped after install;
  whatever protocol it was retransmits on its own and is then switched
  in the dataplane. (DHCP mode keeps replaying the trigger out the
  handoff.)
- **Punt storm protection.** The dataplane suppresses repeat punts per
  tuple for 5 seconds (a rejected or unknown line sending line-rate
  traffic costs one punt per 5 seconds, not one per frame), and the punt
  path is policed globally. Established circuits never punt.
- **Teardown.** With no DHCP lease lifecycle, use `l2gw.idle-timeout`
  (below) as the lease substitute; RADIUS Disconnect-Message and
  operator termination work as in DHCP mode.

### Self-contained mode with local auth

Packet mode with `auth_provider: local` needs no RADIUS and no external
state: the configuration is the database. Every line that appears on an
armed range is authorized, gets the group's default handoff group, an
allocator-assigned egress pair, and its own accounting stream. This is
the zero-touch alternative to a static map when per-line accounting and
per-line teardown still matter. Per-line overrides (a specific line
pinned to a different ISP or egress pair) need RADIUS or per-user local
attributes.

```yaml
aaa:
  auth_provider: local
  policy:
    - name: line-policy
      format: "$subscriber-group$.$svlan$.$cvlan$"
```

## Idle timeout

`l2gw.idle-timeout` (seconds, per subscriber group) tears down a dynamic
circuit after that long without traffic in either direction, freeing its
egress VLAN pair and emitting Accounting-Stop. Counter deltas from the
dataplane are the liveness signal; sweeps run every 30 seconds. `0` or
omitted means circuits are never idle-expired. It applies to both
trigger modes but is the natural lease-substitute for packet mode. The
next frame from the line after an idle teardown simply re-triggers.

## AAA attributes

| Internal attribute | Direction | Meaning |
|---|---|---|
| `l2gw.handoff-group` | Access-Accept | Selects the handoff group by label. Falls back to the subscriber group's `l2gw.handoff-group`. |
| `l2gw.svlan` | Access-Accept | Explicit egress outer VLAN, overriding the allocator. |
| `l2gw.cvlan` | Access-Accept | Explicit egress inner VLAN, overriding the allocator. |
| `l2gw.handoff-group`, `l2gw.svlan`, `l2gw.cvlan` | Accounting | The resolved values are reported back so the OSS learns what was allocated. |

The trigger's Access-Request carries the circuit identity: username
(group-qualified VLAN tuple by default), MAC, S/C VLANs, and access
interface. DHCP-mode triggers additionally contribute DHCPv4 option-82
`circuit_id` / `remote_id` when present; DHCPv6 triggers arriving
relay-encapsulated (an LDRA in the access network) contribute
interface-id (option 18) as `circuit_id` and remote-id (option 37,
enterprise prefix stripped) as `remote_id`. Packet-mode triggers carry
no DHCP options. `aaa-policy` username formats work exactly as for IPoE.

With the RADIUS provider these attributes map to built-in vendor-specific
attributes (OSVBNG-L2GW-Handoff-Group/SVLAN/CVLAN, types 1-3) under the
plugin's `vendor_id`; a matching FreeRADIUS dictionary ships in
`contrib/freeradius/dictionary.osvbng`. The HTTP provider passes the
internal names through verbatim.

## Lifecycle

- **Install.** Trigger, AAA accept, circuit programmed. In DHCP mode the
  held trigger frame is then replayed out the handoff with the
  subscriber's own source MAC so the retail BNG learns the subscriber;
  in packet mode the frame is dropped and the protocol's own retransmit
  takes the now-forwarding circuit.
- **Accounting.** Start on install, interim on the standard cadence with
  per-circuit upstream and downstream counters from the dataplane, Stop on
  teardown. RADIUS and HTTP accounting providers work unchanged.
- **Persistence.** Dynamic circuits survive restarts. They are re-installed
  from the operational DB with no re-authentication and no duplicate
  Accounting-Start.
- **Teardown.** RADIUS Disconnect-Message, operator termination, or
  `l2gw.idle-timeout` expiry. Rejected circuits back off for 30 seconds
  before a retransmit can re-trigger.

CoA policy push is deliberately not supported. Subscriber policy belongs to
the retail ISP's BNG; osvbng is a layer 2 gateway in this role.

## Configuration reload

Committing changes to the `l2gw` block reconciles live state:

- Static maps are diffed against installed static circuits: removed or
  re-pointed maps tear their circuits down, new maps install immediately.
- Egress VLAN allocators are rebuilt from the new handoff group ranges,
  with live circuits' pairs re-marked so no in-use pair is ever
  re-allocated.
- Installed **dynamic** circuits are never touched by config commits:
  they are session state, removed by RADIUS Disconnect-Message or
  operator termination. A dynamic circuit whose handoff group was
  re-pointed keeps forwarding on the old interface until re-established.

## Observability

- `show` path `l2gw.circuits` lists all circuits with access tuple, handoff
  resolution, static or dynamic origin, and state.
- Per-circuit packet/byte counters per direction live in the VPP stats
  segment under `/osvbng/l2gw`.
- Dataplane CLI: `show osvbng l2gw circuits` (vppctl), including counters.

## Full example

```yaml
l2gw:
  handoff-groups:
    isp-blue:
      interface: bond1
      vlan-tpid: dot1ad
      svlan-range: "200-299"
      cvlan-range: "1-4000"
    isp-green:
      interface: eth3
      svlan: 400
      cvlan-range: "1-4000"
  static-maps:
    - access-interface: eth1
      svlan: "10-99"
      handoff-group: isp-green
      transparent: true

subscriber-groups:
  groups:
    wholesale-dynamic:
      vlans:
        - svlan: "100-199"
          cvlan: any
          parent-interface: eth1
          access-types: [l2gw]
          trigger: packet
      l2gw:
        handoff-group: isp-blue
        idle-timeout: 3600
      aaa-policy: line-policy

aaa:
  policy:
    - name: line-policy
      format: "$subscriber-group$.$svlan$.$cvlan$"
      password: wholesale
```
