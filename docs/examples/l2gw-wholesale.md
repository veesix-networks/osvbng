# L2 Wholesale Aggregation (L2GW)

osvbng as a wholesale exchange between independent access network
operators and retail ISPs: each access network delivers its subscribers
as a QinQ NNI on its own port, each ISP takes a handoff on its own port,
and osvbng cross-connects subscriber circuits between the two sides at
layer 2. Circuits are triggered by the line's first frame (DHCP-only or
any-protocol, per range) and RADIUS-driven; the retail ISP's own BNG
terminates the subscriber with its own addressing and policy, whether
that session is DHCP, PPPoE, or static. The exchange operator may own
none of the access infrastructure.

```
 Access Network 1 -----+ eth1                     bond1 +------ ISP X NNI
 (S 100-499,           |                                |       (S 2000-2499)
  C per subscriber)    |                                |
                       |                                |
 Access Network 2 -----+ eth2         osvbng       eth5 +------ ISP Y NNI
 (S 100-499,           |              (l2gw)            |       (S 2000-2499)
  C per subscriber)    |                                |
                       |                                |
 Access Network 3 -----+ eth3                      eth6 +------ ISP Z NNI
 (S 1000-1199)         +--------------------------------+       (S 400 only)
```

## Scenario

- **Access Network 1** and **Access Network 2** sell per-subscriber, and
  their VLAN plans **overlap completely**: line `200.10` exists on both
  footprints as different physical subscribers. That is fine: circuits
  key on the NNI port as well as the tags, and the RADIUS username
  carries the network, so `an1.200.10` and `an2.200.10` are distinct
  lines that can belong to different ISPs.
- **Access Network 2** runs `trigger: packet`: its retail ISPs sell
  PPPoE and static-IP products, so circuits come up on the first frame
  of any protocol instead of requiring DHCP.
- **ISP X** and **ISP Y** both sell on both networks. Their handoffs use
  allocator ranges: each circuit gets its own S/C pair on the NNI.
- **ISP Z** requires a **single fixed S-VLAN (400)** on its core-facing
  NNI, the VLAN-per-ISP interconnect model. Every ISP Z circuit rides
  S 400 and is distinguished by its C-VLAN, allocated per circuit from
  the group's `cvlan-range`.
- **Access Network 3** was bought outright by ISP X: its whole S-VLAN
  range passes transparently with no per-subscriber signalling.

## Configuration

```yaml
interfaces:
  eth1: {}          # Access Network 1 NNI
  eth2: {}          # Access Network 2 NNI
  eth3: {}          # Access Network 3 NNI
  eth5: {}          # ISP Y NNI
  eth6: {}          # ISP Z NNI
  bond1:            # ISP X NNI (LACP)
    bond:
      mode: lacp
      members: [eth7, eth8]

l2gw:
  handoff-groups:
    isp-x:
      interface: bond1
      vlan-tpid: dot1ad
      svlan-range: "2000-2499"
    isp-y:
      interface: eth5
      vlan-tpid: dot1ad
      svlan-range: "2000-2499"
    isp-z:
      interface: eth6
      vlan-tpid: dot1ad
      svlan: 400            # every ISP Z circuit on one S-VLAN
      cvlan-range: "1-4000" # per-circuit inner tag
  static-maps:
    - access-interface: eth3
      svlan: "1000-1199"
      handoff-group: isp-x
      transparent: true

subscriber-groups:
  groups:
    an1:
      vlans:
        - svlan: "100-499"
          cvlan: any
          parent-interface: eth1
          access-types: [l2gw]
      aaa-policy: line
    an2:
      vlans:
        - svlan: "100-499"
          cvlan: any
          parent-interface: eth2
          access-types: [l2gw]
          trigger: packet
      l2gw:
        idle-timeout: 3600
      aaa-policy: line

aaa:
  auth_provider: radius
  policy:
    - name: line
      format: "$subscriber-group$.$svlan$.$cvlan$"
      password: wholesale

plugins:
  subscriber.auth.radius:
    servers:
      - host: 192.0.2.10
        secret: wholesale-secret
```

The username is the line identity: the access network plus the S/C VLAN
tuple its provisioning assigned. `$subscriber-group$` expands to the
group name, so one policy serves every access network. Nothing here
names a subscriber, and the overlapping VLAN plans cannot collide
because the group prefix in the username (and the NNI port in the
dataplane circuit key) disambiguates.

## RADIUS integration

One entry per sold line, written by the exchange operator's BSS when an
ISP places an order. The same access tuple on the two networks, sold to
two different ISPs:

```
an1.200.10  Cleartext-Password := "wholesale"
            OSVBNG-L2GW-Handoff-Group := "isp-x"

an2.200.10  Cleartext-Password := "wholesale"
            OSVBNG-L2GW-Handoff-Group := "isp-y"
```

Both circuits get allocator-assigned egress pairs from their group's
ranges, reported back in accounting.

A line on Access Network 1 sold to ISP Z: the handoff group's fixed
`svlan: 400` pins the outer tag, so only the inner tag is allocated (or
pinned by the BSS if ISP Z pre-provisions their BNG side):

```
an1.301.42  Cleartext-Password := "wholesale"
            OSVBNG-L2GW-Handoff-Group := "isp-z",
            OSVBNG-L2GW-CVLAN := "1042"
```

Moving a line between ISPs is a one-attribute change plus a
Disconnect-Message with the line's Acct-Session-Id; the line's next
trigger frame re-triggers and splices to the new handoff. Ceasing the
line is deleting the entry plus the same Disconnect. On packet-trigger
ranges an idle line also ages out on its own via `idle-timeout`.

The VSA definitions ship in `contrib/freeradius/dictionary.osvbng`.

## Billing feed

Accounting-Start/Interim/Stop per circuit carry the resolved
`OSVBNG-L2GW-Handoff-Group`/`SVLAN`/`CVLAN` plus upstream and downstream
octet and packet counters: who owns the line, on which NNI VLANs, since
when, and how much it moved. The same counters are exported as
`osvbng_dataplane_vpp_l2gw_*` Prometheus metrics labeled by
`handoff_group`, so per-ISP traffic dashboards need no extra probes.

## What each party sees

- **The subscriber**: an ordinary session from their ISP, whether that
  is a DHCP lease, a PPPoE login, or a static assignment.
- **The ISP**: plain QinQ subscribers appearing on one NNI (for ISP Z,
  all inside S-VLAN 400), terminated on their own BNG with their own
  addressing and policy.
- **The access network**: one trunk toward the exchange, no per-ISP
  provisioning ever.
- **The exchange operator**: per-line RADIUS entries and a billing feed;
  osvbng itself is configured once per NNI, never per subscriber.

## Verification

```
osvbngcli show l2gw circuits
sudo docker exec <container> vppctl -s /var/run/osvbng/cli.sock show osvbng l2gw circuits
```
