# L2TP LNS

osvbng acts as the **LNS** — it accepts inbound L2TPv2 tunnels from one
or more LACs, terminates PPP locally, runs LCP and the configured
auth + NCP (IPCP / IPv6CP) layers, and allocates the subscriber's
IPv4 / IPv6 from local pools. From the subscriber's point of view, the
LNS is the BNG: it sees a PPP gateway on the other end of the tunnel.

```
                  PPPoL2TP
LAC (carrier)  ──────────────►  osvbng-LNS  ─────────────────►  core network
              (UDP 1701)
```

The LAC is responsible for terminating the access protocol on its
side (PPPoE typically), looking up tunnel attributes via its own AAA,
and forwarding the subscriber's PPP frames over L2TP. The LNS owns
everything from PPP termination onwards: the subscriber's AAA
exchange (if any), IP allocation, accounting, and forwarding the
subscriber's traffic into the operator's core network.

## Topology

| Node | Role | Interfaces |
|------|------|------------|
| `bng1` | osvbng-LNS | `eth1` (L2TP backbone), `loop100` (subscriber gateway address) |
| `lac` | upstream LAC (xl2tpd / Cisco / Nokia / accel-ppp / RTBrick) | reachable over the L2TP backbone |

## Configuration

```yaml
service-groups:
  lns-default:
    unnumbered: loop100        # per-session vnet ifaces borrow this loopback's IPs

subscriber-groups:
  groups:
    lns:
      access-type: lns
      ipv4-profile: default
      ipv6-profile: default-v6
      default-service-group: lns-default
      aaa-policy: lns-policy
      l2tp:
        profile: LNS_DEFAULT

l2tp:
  profiles:
    LNS_DEFAULT:
      receive-window-size: 16
      hello-interval: 60s
      challenge-required: false   # set true to enforce Challenge-AVP auth
      proxy-lcp-mode: forward     # forward proxy-LCP from ICCN; alternative: renegotiate
      ppp-framing: hdlc           # data-plane PPP framing; default matches pppd / Cisco / Nokia
  peer-policies:
    upstream-wholesale:
      hostname: "lac-wholesale"   # matches the Host Name AVP the LAC sends in SCCRQ
      secret: "shared"
      profile: LNS_DEFAULT
      # ppp-framing: compressed   # override per-peer if a specific LAC uses ACFC

ipv4-profiles:
  default:
    gateway: 100.64.0.1
    dns: [8.8.8.8]
    pools:
      - name: lns-subscribers
        network: 100.64.0.0/16
        priority: 1

ipv6-profiles:
  default-v6:
    iana-pools:
      - name: lns-wan-link
        network: 2001:db8:1::/64
        range_start: 2001:db8:1::1000
        range_end:   2001:db8:1::ffff
    pd-pools:
      - name: lns-pd
        network: 2001:db8:2::/40
        prefix_length: 56

interfaces:
  loop100:
    description: LNS subscriber gateway loopback
    enabled: true
    address:
      ipv4: ["100.64.0.1/32"]
      ipv6: ["2001:db8:1::1/128"]

aaa:
  auth_provider: local
  policy:
    - name: lns-policy
      type: ppp
      authenticate: true          # set false for wholesale (skip LNS auth verification)
      max_concurrent_sessions: 1
```

## Authentication

PPP authentication at the LNS is optional and driven by the AAA policy
attached to the subscriber group:

- `authenticate: true` — the LNS runs the auth protocol the LAC
  proxied (PAP or CHAP, per RFC 2868 Proxy-Authen AVPs in ICCN) and
  validates the credentials against the configured auth provider
  (local DB, RADIUS, …).
- `authenticate: false` — the LNS skips auth verification entirely and
  proceeds straight to NCP. Useful for wholesale deployments where
  the LAC has already authenticated and the LNS just terminates the
  session.

A LAC may also send no Proxy-Authen at all; the LNS will then either
negotiate an auth protocol of its own choosing, or skip auth, per
`authenticate`.

## Per-session interface

Every PPPoL2TP session lives on its own dataplane interface that the
LNS creates at ICRQ time. ACLs / QoS / counters attach to it exactly
the same way they do for PPPoE and IPoE sessions — operationally,
"this is a subscriber session" is the only thing that matters; the
fact that it rides L2TP is a transport detail.

## `ppp-framing`

The data-plane PPP framing for each session is resolved once at session
create from operator config (per-peer override → profile default →
`hdlc`). The plugin stores a single byte (`ppp_hdr_skip` = 0 or 2) on
the session struct and the data path reads it with no per-packet
detection of the on-wire bytes.

- `hdlc` — Address + Control prefix (`0xff 0x03`) on every data frame.
  This is the default and matches **every** major LNS in the wild
  (Cisco, Nokia, RTBrick, accel-ppp, xl2tpd's pppd).
- `compressed` — ACFC negotiated, no prefix. Use only when you know
  every LAC under this profile / peer policy actually negotiated ACFC.

## Show

| Command | What it shows |
|---------|----------------|
| `show l2tp tunnels` | One row per inbound tunnel: local + peer IP, local + peer Tunnel-ID, Role=LNS, FSM state, bound session count. |
| `show subscriber sessions` | Subscriber view with the negotiated IPv4 / IPv6 / PD, AAA session id, allocated pool, per-session `IfIndex`. |

## See also

- [L2TP LAC (Wholesale)](l2tp-lac.md) — the inverse scenario where osvbng forwards subscribers to a remote LNS
- [L2TP configuration reference](../configuration/l2tp.md)
