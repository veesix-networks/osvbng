# Configuration Examples

Real-world configuration examples for common ISP deployment scenarios. Each example is self-contained and can be adapted to your environment by replacing ASNs, prefixes, and community values.

## BNG Deployments

- [IPoE BNG](ipoe-bng.md) — Dual-stack IPoE with local auth, DHCP/DHCPv6, OSPF/BGP
- [PPPoE BNG](pppoe-bng.md) — Dual-stack PPPoE with per-subscriber authentication
- [IPoE + PPPoE BNG](ipoe-pppoe-bng.md) — Combined deployment serving both access types on the same node
- [L2TP LAC (Wholesale)](l2tp-lac.md) — PPPoE termination + L2TPv2 tunnelling to a remote LNS for wholesale ISP deployments
- [VXLAN Access (PWHE)](pwhe-access.md) — IPoE, PPPoE, and LAC subscribers arriving over a VXLAN access network via a pseudowire headend

## Fabric & Overlay

- [EVPN-VXLAN Fabric](evpn-fabric.md) — osvbng as a fabric VTEP: EVPN-discovered tunnels, anycast-VTEP HA with route-driven failover, vendor interop notes

## Policy & Filtering

- [Routing Policies](routing-policies.md) — RTBH, bogon filtering, geographic communities, peering/transit policies, AS-path filtering, outbound scrubbing
