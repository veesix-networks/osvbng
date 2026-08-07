# L2GW (Layer 2 Wholesale Gateway)

The layer 2 wholesale gateway cross-connects subscriber circuits between
access networks and retail ISP handoff ports without terminating DHCP or L3.
osvbng acts as the wholesale aggregator: the first DHCPv4 DISCOVER or DHCPv6
SOLICIT on a wholesale circuit triggers AAA, the auth response selects a named
**handoff group** (plus optional explicit egress VLANs), and osvbng installs a
bidirectional L2 cross-connect with S/C-VLAN rewrite. From then on every frame
(ARP, ND, DHCP renewals, IGMP) is switched in the dataplane, and the retail
ISP's BNG terminates the subscriber. It is the IPoE analogue of LAC/LNS
wholesale.

Two operating modes share one mechanism:

- **Static maps.** An entire access S-VLAN (all C-VLANs) is cross-connected
  to an ISP by configuration. No trigger, no RADIUS, no per-subscriber state.
- **Dynamic circuits.** Per-(S,C) circuits authorized by AAA, with egress
  VLANs allocated by osvbng or returned by RADIUS.

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
      l2gw:
        handoff-group: isp-blue   # default when AAA returns no label
      aaa-policy: line-policy  # e.g. format "$svlan$.$cvlan$", the line's VLAN tuple
```

## AAA attributes

| Internal attribute | Direction | Meaning |
|---|---|---|
| `l2gw.handoff-group` | Access-Accept | Selects the handoff group by label. Falls back to the subscriber group's `l2gw.handoff-group`. |
| `l2gw.svlan` | Access-Accept | Explicit egress outer VLAN, overriding the allocator. |
| `l2gw.cvlan` | Access-Accept | Explicit egress inner VLAN, overriding the allocator. |
| `l2gw.handoff-group`, `l2gw.svlan`, `l2gw.cvlan` | Accounting | The resolved values are reported back so the OSS learns what was allocated. |

The trigger's Access-Request carries the usual circuit identity: MAC, S/C
VLANs, access interface, and DHCPv4 option-82 `circuit_id` / `remote_id`
when present. DHCPv6 triggers arriving relay-encapsulated (an LDRA in the
access network) contribute interface-id (option 18) as `circuit_id` and
remote-id (option 37, enterprise prefix stripped) as `remote_id`.
`aaa-policy` username formats work exactly as for IPoE.

With the RADIUS provider these attributes map to built-in vendor-specific
attributes (OSVBNG-L2GW-Handoff-Group/SVLAN/CVLAN, types 1-3) under the
plugin's `vendor_id`; a matching FreeRADIUS dictionary ships in
`contrib/freeradius/dictionary.osvbng`. The HTTP provider passes the
internal names through verbatim.

## Lifecycle

- **Install.** Trigger, AAA accept, circuit programmed. The held trigger
  frame is then replayed out the handoff with the subscriber's own source
  MAC so the retail BNG learns the subscriber.
- **Accounting.** Start on install, interim on the standard cadence with
  per-circuit upstream and downstream counters from the dataplane, Stop on
  teardown. RADIUS and HTTP accounting providers work unchanged.
- **Persistence.** Dynamic circuits survive restarts. They are re-installed
  from the operational DB with no re-authentication and no duplicate
  Accounting-Start.
- **Teardown.** RADIUS Disconnect-Message or operator termination. Rejected
  circuits back off for 30 seconds before a retransmit can re-trigger.

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
      l2gw:
        handoff-group: isp-blue
      aaa-policy: line-policy
```

## VXLAN overlay NNIs

An NNI does not have to be a physical port: any [VXLAN tunnel interface](interfaces.md#vxlan) works as an l2gw `parent-interface` or `handoff-groups.<name>.interface` with no additional l2gw configuration. This is the building block for fabric-scale aggregation, where each access-operator or ISP E-NNI arrives as a point-to-point E-LINE service (one VNI per NNI) across a spine-leaf underlay. Because every NNI gets its own tunnel, each one carries a full independent S-VLAN namespace: two access operators can both deliver S-VLAN 100 without translation.

```yaml
interfaces:
  loop0:
    enabled: true
    address:
      ipv4: [10.254.0.1/32]       # VTEP loopback
  vxlan-an1:
    description: Access operator 1 NNI
    enabled: true
    vxlan: { src-interface: loop0, dst: 10.254.0.101, vni: 10101 }
  vxlan-isp-blue:
    description: ISP blue handoff
    enabled: true
    vxlan: { src-interface: loop0, dst: 10.254.0.102, vni: 20101 }

l2gw:
  handoff-groups:
    isp-blue:
      interface: vxlan-isp-blue
      vlan-tpid: dot1q
      svlan-range: "200-299"
      cvlan-range: "1-4000"

subscriber-groups:
  groups:
    wholesale:
      vlans:
        - svlan: "100-199"
          cvlan: any
          parent-interface: vxlan-an1
          access-types: [l2gw]
      l2gw:
        handoff-group: isp-blue
      aaa-policy: line-policy
```

The dataplane requires the `osvbng_tunnel` VPP plugin (shipped alongside the l2gw plugin): it re-injects decapsulated frames into the standard RX feature pipeline, so trigger snooping, circuit switching, counters, and restart restore behave identically to physical NNIs.

### HA: VTEP failover by routing

With HA enabled, advertise the VTEP loopback from the active node only by listing it in the SRG's `networks`. On promotion the /32 is advertised into BGP (and withdrawn on demotion), so the fabric steers every tunnel to whichever node is active — no MAC dance, consistent with the existing failover-by-routing design. Standby pre-installed circuits are ready when traffic arrives.

```yaml
ha:
  enabled: true
  srgs:
    srg1:
      networks:
        - prefix: 10.254.0.1/32   # VTEP loopback, advertised while ACTIVE
```

For active/active, use two VTEP loopbacks with opposite-priority SRGs and split the NNI tunnels between them.

### Underlay MTU

The tunnel adds ~50 bytes on top of QinQ subscriber frames. Run a jumbo underlay (`mtu: 9000` on the underlay interfaces and fabric-wide). Encap uses flow-hash UDP source ports, so ECMP paths, LAG members, and receiver RSS queues all load-balance per subscriber flow.
