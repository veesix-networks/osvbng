# 30-l2tp-lns: two LACs into one osvbng LNS

End-to-end test of the L2TPv2 LNS path. Two external LACs dial in to
osvbng-LNS, which terminates PPP, runs LCP / CHAP / IPCP / IPv6CP
locally, allocates the subscriber's IPv4 + IPv6 from local pools, and
binds a per-session VPP interface so subscriber traffic transits the
dataplane in both directions.

- **xl2tpd + pppd**: one tunnel, one session. Keeps a real pppd in the
  interop set.
- **bngblaster PPPoL2TP LAC** (BNG Blaster 1.0.0): one tunnel, ten
  sessions. This is the scale shape of an LNS: every session from a LAC
  arrives inside the same UDP flow.

## Topology

```
+-----+        +-------------+        +------+
| lac |--eth1--| corerouter1 |--eth2--| bng1 |   osvbng LNS
+-----+        |   (FRR)     |        +------+
 10.10.0.2/30  |             |         10.20.0.2/30
               |             |         loop0 10.254.0.1
+---------+    |             |         loop100 100.64.0.1 + 2001:db8:1::1
| blaster |-e1-|             |
+---------+    +-------------+
 10.30.0.2/30   lo 10.254.0.2/32
```

- **lac**: xl2tpd + pppd container (see `test-infra/xl2tpd/`). Auto-dials
  on startup to `lns 10.20.0.2`, CHAP user `user1` / password `test`.
- **blaster**: `veesixnetworks/bngblaster:1.0.0`. Access type `pppol2tp`
  with one `l2tp-client` named `blaster`, ten sessions, CHAP users
  `blaster1..blaster10`, IPv4 session traffic toward its own network
  interface address. Config in `config/blaster/config.json`.
- **corerouter1**: FRR OSPF router. Static route `100.64.0.0/16 via
  10.20.0.2` so subscriber-pool reply traffic can transit it.
- **bng1**: osvbng-LNS. Loopback `loop100` carries the LNS subscriber
  gateway IPs (matching the IPv4 profile gateway + IPv6 IANA gateway).
  Per-session VPP interface gets set unnumbered to `loop100`. One
  peer-policy per LAC hostname (`lac`, `blaster`).

## Auth model

osvbng-LNS terminates PPP locally. Subscriber CHAP runs against the
local auth provider (`subscriber.auth.local`), which is configured with
`allow_all: true` for this test, so any username succeeds. Real
deployments would replace this with a populated user store or RADIUS.

## Punt policer

The osvbng config raises the L2TP punt policer well above its default.
The policer divides the configured rate across every VPP worker, and a
LAC's whole session population shares one UDP flow pinned to one
worker, so the default 500 pps is about 24 pps for that LAC on a
21-worker box. That is low enough to police ICCN and LCP during a
ten-session bring-up. The override keeps the suite measuring the LNS
rather than the policer; the comment in `config/bng1/osvbng.yaml`
carries the detail.

## What this exercises

- L2TPv2 control: SCCRQ, SCCRP, SCCCN, ICRQ, ICRP, ICCN, periodic
  Hellos and ZLB acks, from two LACs with different implementations
- PPP control plane terminated at the LNS: LCP (HDLC Address and
  Control bytes on every control frame the LNS sends, which bngblaster
  requires and pppd tolerates), CHAP-MD5, IPCP, IPv6CP, LCP Echo
- AAA event for every subscriber, allow_all branch
- IPv4 allocation from `100.64.0.0/16`, IPv6 IANA from `2001:db8:1::/64`,
  IPv6 PD from `2001:db8::/40`, eleven sessions
- VPP `AddPPPoL2TPSession` creating the per-session vnet interface
  (`l2tpv2_sessionN`) at ICRQ time, with `ppp_hdr_skip=2` resolved from
  the peer-policy / profile config (HDLC default)
- `SetUnnumbered` to the LNS gateway loopback `loop100`
- `PPPoL2TPSetSubscriberIPv4 / _IPv6 / _SetDelegatedPrefix` installing
  the subscriber routes via the per-session iface, FIB-tracked
- `vnet_set_interface_l3_output_node(..., "tunnel-output")` so egress
  packets resolved via the per-session iface go through the midchain
  rewrite (IP + UDP + L2TP + HDLC + PPP proto field)
- pppd subscriber pings the local LNS gateway (`100.64.0.1`), the BNG
  loopback (`10.254.0.1`) and the corerouter1 loopback (`10.254.0.2`):
  local termination and transit, both directions
- bngblaster upstream session traffic from all ten sessions verified at
  its network interface after transiting osvbng
- bngblaster downstream delivery: corerouter1 pings every session and
  the session's ICMP receive counter proves arrival through the tunnel

## Known limits of the LAC emulation

BNG Blaster 1.0.0 builds no downstream session traffic for PPPoL2TP
sessions and sends no ICMP echo reply from them, so downstream is
proven by the receive counter rather than a round trip. Upstream
session traffic must be given an explicit `ipv4-address`, since the
PPPoL2TP stream builder does not default to the network interface
address.

## Running

```sh
sudo make robot-test suite=30-l2tp-lns
```
