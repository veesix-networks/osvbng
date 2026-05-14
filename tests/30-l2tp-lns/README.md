# 30-l2tp-lns — xl2tpd-LAC ↔ osvbng-LNS

End-to-end test of the L2TPv2 LNS path. An external LAC (xl2tpd running
pppd) dials in to osvbng-LNS, which terminates PPP, runs LCP / CHAP /
IPCP / IPv6CP locally, allocates the subscriber's IPv4 + IPv6 from local
pools, and binds the per-session VPP interface so subscriber traffic
transits the dataplane in both directions.

## Topology

```
+-----+        +-------------+        +------+
| lac |--eth1--| corerouter1 |--eth2--| bng1 |   osvbng LNS
|     |        |   (FRR)     |        |      |
+-----+        +-------------+        +------+
 10.10.0.2/30   10.10.0.1/30           10.20.0.2/30
                10.20.0.1/30           loop0 10.254.0.1
                lo    10.254.0.2/32    loop100 100.64.0.1 + 2001:db8:1::1
```

- **lac**: lightweight xl2tpd + pppd container (see `test-infra/xl2tpd/`).
  Auto-dials on startup to `lns 10.20.0.2`, CHAP user `user1` / password
  `test`.
- **corerouter1**: FRR OSPF router. Static route `100.64.0.0/16 via
  10.20.0.2` so subscriber-pool reply traffic can transit it.
- **bng1**: osvbng-LNS. Loopback `loop100` carries the LNS subscriber
  gateway IPs (matching the IPv4 profile gateway + IPv6 IANA gateway).
  Per-session VPP interface gets set unnumbered to `loop100`.

## Auth model

osvbng-LNS terminates PPP locally. Subscriber CHAP runs against the
local auth provider (`subscriber.auth.local`), which is configured with
`allow_all: true` for this test — any username succeeds. Real
deployments would replace this with a populated user store or RADIUS.

## What this exercises

- L2TPv2 control: SCCRQ → SCCRP → SCCCN, ICRQ → ICRP → ICCN, periodic
  Hellos and ZLB acks
- PPP control plane terminated at the LNS: LCP (with ACFC=off so HDLC
  prefix stays on the wire), CHAP-MD5, IPCP, IPv6CP — including the LCP
  Echo Reply that pppd uses for keepalive
- AAA event for the subscriber, allow_all branch
- IPv4 allocation from `100.64.0.0/16`, IPv6 IANA from `2001:db8:1::/64`,
  IPv6 PD from `2001:db8::/40`
- VPP `AddPPPoL2TPSession` creating the per-session vnet interface
  (`l2tpv2_session0`) at ICRQ time, with `ppp_hdr_skip=2` resolved from
  the peer-policy / profile config (HDLC default)
- `SetUnnumbered` to the LNS gateway loopback `loop100` — enables IPv4 /
  IPv6 on the per-session iface as a side effect of
  `vnet_sw_interface_update_unnumbered`
- `PPPoL2TPSetSubscriberIPv4 / _IPv6 / _SetDelegatedPrefix` installing
  the subscriber routes via the per-session iface, FIB-tracked
- `vnet_set_interface_l3_output_node(..., "tunnel-output")` so egress
  packets resolved via the per-session iface go through the midchain
  rewrite (IP + UDP + L2TP + optional HDLC + PPP proto field)
- Subscriber pings local LNS gateway (`100.64.0.1`) and BNG loopback
  (`10.254.0.1`) — proves local-termination both directions
- Subscriber transit ping to `10.254.0.2` (corerouter1 loopback, beyond
  osvbng) — proves bidirectional dataplane through osvbng-LNS to a
  destination outside the BNG itself

## Running

```sh
sudo make robot-test suite=30-l2tp-lns
```
