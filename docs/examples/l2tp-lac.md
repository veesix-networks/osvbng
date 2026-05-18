# L2TP LAC (Wholesale)

A wholesale BNG deployment where osvbng acts as **LAC**: it terminates PPPoE
locally, authenticates the subscriber against the local user store (or RADIUS
backend) only enough to look up an LNS endpoint, and forwards the subscriber's
PPP frames over an L2TPv2 tunnel to the remote LNS. The LNS terminates PPP,
hands out the IP, and owns the subscriber's data plane from that point on.

```
            PPPoE / Q-in-Q                              PPPoL2TP
subscriber  ───────────────►   osvbng-LAC   ────────────────────────►   LNS (ISP)
                              (eth1 access)      (eth2 backbone, UDP 1701)
```

The LAC's AAA policy looks up the subscriber by `agent-remote-id` and returns
RFC 2868 `Tunnel-*` attributes that pick the right LNS. The actual subscriber
authentication (CHAP) is forwarded to the LNS via proxy-auth AVPs in ICCN —
osvbng never validates the subscriber's password.

## Topology

| Node | Role | Interfaces |
|------|------|------------|
| `bng1` | osvbng-LAC | `eth1` (subscriber, Q-in-Q), `eth2` (L2TP backbone) |
| `lns` | LNS (e.g. bngblaster, Cisco, Nokia) | reachable at `10.0.0.2` |

## Configuration

```yaml
subscriber-groups:
  groups:
    pppoe-lac:
      access-types: [lac]
      vlan-tpid: dot1q
      vlans:
        - svlan: "200-210"
          cvlan: any
          interface: loop100
          parent-interface: eth1
      aaa-policy: pppoe-policy
      l2tp:
        profile: L2TP_LAC_DEFAULT

l2tp:
  tunnel-pools:
    LNS_POOL:
      local-name: bng1
      lns:
        - name: lns-provider1
          ipv4: 10.0.0.2
          source-ipv4: 10.0.0.1
          secret: shared
          preference: 100
  profiles:
    L2TP_LAC_DEFAULT:
      tunnel-pool: LNS_POOL
      session-limit: 1000
      hello-interval: 60s
      receive-window-size: 16
      max-attempts-per-subscriber: 4

aaa:
  auth_provider: local
  nas_identifier: osvbng
  policy:
    - name: pppoe-policy
      type: ppp
      format: $agent-remote-id$
      authenticate: false
      max_concurrent_sessions: 1

interfaces:
  loop0:
    description: Control Plane Loopback
    enabled: true
    address:
      ipv4:
        - 10.254.0.1/32
  eth1:
    description: Subscriber-facing (PPPoE)
    enabled: true
  eth2:
    description: L2TP backbone (LNS reachability)
    enabled: true
    address:
      ipv4:
        - 10.0.0.1/30
  loop100:
    description: Subscriber Gateway Loopback
    enabled: true
    address:
      ipv4:
        - 10.255.0.1/32
```

The LAC strips Ethernet, VLAN(s), and the PPPoE header before encapsulating
the PPP frame in L2TP. The LNS sees only the PPP frame (`HDLC Address +
Control + PPP protocol + payload`) — no Ethernet, no VLAN tags, no PPPoE
session ID. Subscriber circuit context that the LNS needs for billing or
service selection must be passed via the proxy-auth AVPs in ICCN, the
`Tunnel-Calling-Station-ID` / `Tunnel-Called-Station-ID` AVPs, or RADIUS
attributes the LNS will look up against its own AAA — the L2TP data frame
itself does not carry the access-side L2.

## Provisioning subscribers

For local auth, create the user and stamp the Tunnel-* attributes on it.
The `agent-remote-id` from the PPPoE Vendor-Specific Tags is the lookup key
per the `format: $agent-remote-id$` policy:

```bash
curl -X POST http://${BNG}:8080/api/exec/subscriber/auth/local/users/create \
  -H "Content-Type: application/json" \
  -d '{"username":"user1","enabled":true}'

curl -X POST http://${BNG}:8080/api/exec/subscriber/auth/local/user/1/attribute \
  -H "Content-Type: application/json" \
  -d '{"attribute":"tunnel.type","value":"L2TP","op":"set"}'

curl -X POST http://${BNG}:8080/api/exec/subscriber/auth/local/user/1/attribute \
  -H "Content-Type: application/json" \
  -d '{"attribute":"tunnel.medium-type","value":"IPv4","op":"set"}'

curl -X POST http://${BNG}:8080/api/exec/subscriber/auth/local/user/1/attribute \
  -H "Content-Type: application/json" \
  -d '{"attribute":"tunnel.server-endpoint","value":"10.0.0.2","op":"set"}'

curl -X POST http://${BNG}:8080/api/exec/subscriber/auth/local/user/1/attribute \
  -H "Content-Type: application/json" \
  -d '{"attribute":"tunnel.password","value":"shared","op":"set"}'
```

For RADIUS, the Access-Accept returns the same attributes (tagged per
RFC 2868 §3.4 if multiple candidate LNSs are offered):

```
Tunnel-Type:1 += L2TP,
Tunnel-Medium-Type:1 += IP,
Tunnel-Client-Endpoint:1 += 10.0.0.1,
Tunnel-Server-Endpoint:1 += 10.0.0.2,
Tunnel-Password:1 += shared,
Tunnel-Preference:1 += 100,

Tunnel-Type:2 += L2TP,
Tunnel-Medium-Type:2 += IP,
Tunnel-Client-Endpoint:2 += 10.0.0.1,
Tunnel-Server-Endpoint:2 += 10.0.0.3,
Tunnel-Password:2 += shared,
Tunnel-Preference:2 += 200,
```

## Flow

1. Subscriber sends PPPoE Discovery → osvbng sends PADO/PADS.
2. LCP converges between subscriber and osvbng.
3. CHAP-Response from subscriber → osvbng publishes an AAA-request.
4. AAA reply returns the `Tunnel-*` attributes → osvbng selects the first
   non-denylisted LNS in preference order.
5. osvbng opens an L2TPv2 tunnel: SCCRQ → SCCRP → SCCCN.
6. osvbng opens an L2TPv2 session: ICRQ → ICRP → ICCN, with proxy-LCP
   (`Last-Sent-LCP-CONFREQ`, `Last-Received-LCP-CONFREQ`) and proxy-auth
   (`Proxy-Authen-Type`, `Proxy-Authen-Name`, `Proxy-Authen-Challenge`,
   `Proxy-Authen-Response`) AVPs.
7. CHAP-Success forwarded to the subscriber.
8. PPP IPCP/IP6CP between subscriber and LNS — bridged through the tunnel
   by the VPP dataplane. The subscriber receives its IP from the LNS pool.

The osvbng session state is `tunneled` for the lifetime of the bridge:

```bash
$ curl -sf http://${BNG}:8080/api/show/subscriber/sessions | jq
[
  {
    "SessionID": "...",
    "State": "tunneled",
    "Username": "user1",
    "TunneledToLNS": "10.0.0.2",
    ...
  }
]
```

## Operational notes

- **Source IP**: every LAC must source its L2TP traffic from a routable IP
  the LNS can reply to. Set `tunnel-pools.<pool>.lns.<i>.source-ipv4`
  per-LNS, or let RADIUS override per-subscriber via `Tunnel-Client-Endpoint`.
- **VRF source**: when the L2TP backbone is in a non-default VRF, set
  `tunnel-pools.<pool>.lns.<i>.vrf` per LNS entry. The Linux Control Plane
  TAP for that interface anchors the kernel-UDP socket.
- **Denylist**: a tunnel that fails (CDN result-code 2/4/5/6/10, or
  retransmit timeout) is excluded for `denylist.tunnel-ttl`. Configure
  multiple LNS candidates with different `preference` values to ride
  through outages.
- **PPP HDLC framing**: osvbng prepends Address (`0xff`) and Control (`0x03`)
  octets on L2TP-carried PPP frames per the PPP default (RFC 1661 §6.6).
  This matches every major LNS implementation; ACFC negotiation is not yet
  honoured.

## See also

- [L2TP configuration reference](../configuration/l2tp.md)
- [PPPoE BNG example](pppoe-bng.md)
- RFC 2661, RFC 2868, RFC 3437
