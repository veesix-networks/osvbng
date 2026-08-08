# EVPN-VXLAN Fabric

osvbng as a VTEP in a spine-leaf EVPN-VXLAN fabric: access operator
NNIs and ISP handoffs arrive as VXLAN services instead of physical
ports, and BGP EVPN replaces per-tunnel remote-VTEP provisioning with
discovery. Every service osvbng offers over physical ports works
unchanged over the discovered tunnels: l2gw wholesale circuits attach
to the tunnel directly, and IPoE, PPPoE, and L2TP LAC terminate on a
pseudowire headend riding it.

```
                       +--------- spine/leaf fabric ---------+
 Access Operator ------+ leaf1                               |
 (QinQ NNI,            |   VNI 10101 <-- EVPN --> anycast    |
  ESI-LAG or           |                          VTEP       |
  anycast VTEP)        |                        10.254.1.1   |
                       |                        /         \  |
 ISP Handoff ----------+ leaf2                bng1        bng2
 (QinQ NNI)            |   VNI 10201       (ACTIVE)    (STANDBY)
                       +-------------------------------------+
```

Each NNI is a point-to-point E-LINE service: one VNI per NNI, tags
transparent, no MAC learning in the fabric. Fabric state scales with
NNIs, not subscribers - subscriber MACs live only in osvbng's circuit
tables and the retail BNGs. This is why osvbng needs no dynamic-VXLAN
dataplane extensions: per VNI, EVPN answers exactly one question (the
remote VTEP), and the answer programs a standard point-to-point tunnel.

## How discovery works

For every `signaling: evpn` tunnel, osvbng maintains a non-forwarding
kernel VXLAN mirror device in the dataplane namespace. FRR derives the
VNI advertisement from it (type-3 IMET routes) and installs learned
remote VTEPs back onto it; osvbng consumes those and programs the real
VPP tunnel. The tunnel exists from boot against a blackhole placeholder
destination, so everything stacked on it - pseudowire binds,
subinterfaces, service arming - is wired before discovery completes;
learning the remote VTEP just makes it forward. Withdrawal reverts the
tunnel to the placeholder, which behaves as a down transport.

## Configuration

The underlay runs BGP with the fabric (jumbo MTU end to end: VXLAN adds
~50 bytes on top of QinQ payloads). The overlay needs only the
`l2vpn-evpn` address family and per-tunnel `signaling: evpn`:

```yaml
interfaces:
  eth1:
    description: Fabric underlay
    enabled: true
    mtu: 9000
    address:
      ipv4: [10.98.1.1/24]
  loop0:
    description: VTEP loopback
    enabled: true
    address:
      ipv4: [10.254.1.1/32]
  vxlan-an1:
    description: Access operator NNI (EVPN-signaled)
    enabled: true
    vxlan:
      src-interface: loop0
      vni: 10101
      signaling: evpn
  pw-an1:
    description: Access operator pseudowire headend
    enabled: true
    mtu: 1508
    pseudowire:
      transport: vxlan-an1

protocols:
  bgp:
    asn: 65000
    router-id: 10.253.1.1
    neighbors:
      10.98.1.2:
        remote-as: 65000
    ipv4-unicast: {}
    l2vpn-evpn:
      advertise-all-vni: true
      neighbors:
        10.98.1.2: {}
```

No `dst` anywhere: the remote VTEP for VNI 10101 is learned from the
leaf's type-3 route. Services reference `vxlan-an1` (l2gw handoff
groups, subscriber-group parent interfaces) or `pw-an1` (IPoE / PPPoE /
LAC termination) exactly as in the physical-port examples.

## HA: anycast VTEP with route-driven failover

Both nodes carry the **same** VTEP loopback and identical tunnel
config. The fabric sees one logical VTEP: both nodes advertise the VNI,
both discover the leaf, and both hold fully programmed tunnels and
bound headends - the standby's dataplane is ready before promotion.
Which node actually receives traffic is decided purely by underlay
routing: the SRG injects the anycast /32 on the ACTIVE and withdraws it
on demotion.

```yaml
ha:
  enabled: true
  node_id: bng-1
  peer:
    address: bng2.example.net:50051
  srgs:
    default:
      virtual_mac: "02:00:5e:00:01:01"
      priority: 100
      subscriber_groups: [default]
      networks:
        - prefix: 10.254.1.1/32
```

Pin the pseudowire headend `mac-address` to the SRG virtual MAC on both
nodes so the subscriber-facing gateway MAC never changes across
switchover. On graceful switchover the /32 moves via BGP
withdraw/advertise, the fabric's encapsulation follows the route, and
synced sessions forward through the new active's own tunnel - no GARP,
no fabric reconfiguration.

!!! warning "Never redistribute the VTEP loopback"
    The anycast /32 must enter BGP **only** through the SRG `networks`
    injection. If `redistribute connected` covers the VTEP loopback,
    both nodes advertise it permanently and failover steering breaks.
    Keep the loopback out of any redistribution, or filter it with a
    route-policy.

## Interoperating with vendor fabrics

osvbng consumes type-3 (IMET) routes with ingress replication - the
baseline every mainstream NOS originates for a bridged L2 VNI. Notes
for real fabrics:

- **Set explicit route-targets per EVI** so both sides agree
  (`protocols.bgp.l2vpn-evpn` neighbors plus FRR's per-VNI `rd` /
  `route-target` overrides render from the config); auto-derived RTs
  differ across vendors and ASNs.
- **One remote VTEP per VNI.** A dual-homed NNI on a leaf pair must
  present a shared anycast service VTEP (MLAG/vPC-style, universally
  supported); underlay ECMP spreads flows across the pair using the
  encap's flow-hash UDP source ports. Fabrics that cannot do anycast
  VTEP should single-home the NNI or split it into two NNIs.
- **Multicast-underlay BUM is not supported** - use ingress
  replication for the service VNIs facing osvbng.
- **E-LAN (shared bridge domains) and ESI all-active toward osvbng are
  deliberately out of scope**: the wholesale gateway model keeps
  subscriber MACs out of the fabric, which is what lets the fabric
  scale with NNIs instead of subscribers.
