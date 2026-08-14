# Configuration Examples

Real-world configuration examples for common ISP deployment scenarios. Each example is self-contained and can be adapted to your environment by replacing ASNs, prefixes, and community values.

## BNG Deployments

- [IPoE BNG](ipoe-bng.md) - dual-stack IPoE with local auth, DHCP/DHCPv6, OSPF/BGP
- [PPPoE BNG](pppoe-bng.md) - dual-stack PPPoE with per-subscriber authentication
- [IPoE + PPPoE BNG](ipoe-pppoe-bng.md) - combined deployment serving both access types on the same node
- [L2TP LAC (Wholesale)](l2tp-lac.md) - PPPoE termination and L2TPv2 tunnelling to a remote LNS for wholesale ISP deployments
- [L2TP LNS](l2tp-lns.md) - inbound L2TPv2 tunnels from remote LACs, local PPP termination, auth, and IP allocation
- [L2 Wholesale (L2GW)](l2gw-wholesale.md) - layer 2 cross-connect between access network NNIs and retail ISP handoffs, RADIUS-driven per line
- [VXLAN Access (PWHE)](pwhe-access.md) - IPoE, PPPoE, and LAC subscribers arriving over a VXLAN access network via a pseudowire headend

## VRFs & L3VPN

- [VRF-Lite](vrf-lite.md) - two customer VRFs on one BNG, per-VRF core sub-interfaces and iBGP sessions, no MPLS
- [MPLS L3VPN](l3vpn.md) - VPNv4 iBGP between PEs with LDP transport labels, subscribers in a customer VRF

## Fabric & Overlay

- [EVPN-VXLAN Fabric](evpn-fabric.md) - osvbng as a fabric VTEP: EVPN-discovered tunnels, anycast-VTEP HA with route-driven failover, vendor interop notes

## Policy & Filtering

- [Routing Policies](routing-policies.md) - RTBH, bogon filtering, geographic communities, peering/transit policies, AS-path filtering, outbound scrubbing
