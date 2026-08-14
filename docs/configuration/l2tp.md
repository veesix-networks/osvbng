# L2TP

L2TPv2 (RFC 2661) configuration. osvbng supports both roles:

- **LAC**: terminate PPPoE locally and tunnel the subscriber's PPP frames over
  L2TP to a remote LNS.
- **LNS**: accept tunnels from remote LACs, terminate the PPP session, and
  address the subscriber from local pools.

The block configures three things: per-LNS endpoint pools (`tunnel-pools`,
LAC-only), behavioural profiles (`profiles`), and authorization for inbound LAC
peers (`peer-policies`, LNS-only). A subscriber group selects the role through
its `access-types` and binds a profile through `l2tp.profile`.

## `l2tp.tunnel-pools`

A named catalogue of LNS endpoints the LAC tries in preference order when
selecting a tunnel for a subscriber.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `local-name` | string | Host Name AVP value sent in SCCRQ. Defaults to the BNG hostname when empty. | `bng1` |
| `lns` | [[LNSRef](#lnsref)] | Ordered list of LNS endpoints. | |

### LNSRef

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `name` | string | Identifier for show commands. | `lns-provider1` |
| `ipv4` | string | LNS IPv4 address. | `10.0.0.2` |
| `source-ipv4` | string | Local IPv4 used as the L2TP tunnel source (Cisco `source-ip`, RTBrick `client-ipv4`). Required when AAA does not return `Tunnel-Client-Endpoint`. | `10.0.0.1` |
| `secret` | string | Shared secret for Challenge/Challenge-Response AVPs. Empty disables tunnel auth. | `s3cret` |
| `preference` | uint16 | Lower wins. Tied to RFC 2868 Tunnel-Preference. | `100` |
| `vrf` | string | VRF to source the L2TP backbone in. Defaults to the global table. | `wholesale` |
| `ppp-framing` | string | Override the profile's `ppp-framing` for sessions opened toward this specific LNS. Useful when one upstream wholesale operator expects ACFC compressed framing and another expects HDLC on the same profile. | `hdlc` |

AAA-returned `Tunnel-Client-Endpoint` (RFC 2868) takes precedence over
`source-ipv4` per LNS.

## `l2tp.profiles`

Profiles bundle timers, limits, and authentication policy. A subscriber
group references one profile via `l2tp.profile`.

| Field | Type | Description | Default |
|-------|------|-------------|---------|
| `session-limit` | int | Max concurrent sessions per profile. | unlimited |
| `hello-interval` | duration | Time between L2TP HELLO keepalives. | `60s` |
| `receive-window-size` | int | Advertised RWS in SCCRQ/SCCRP. | `4` |
| `df-bit` | bool | Set DF in the outer IP header of L2TP frames. | `false` |
| `tunnel-pool` | string | Name of the `tunnel-pools` entry to draw from. | — |
| `retransmit` | [Retransmit](#retransmit) | Control-channel retransmit knobs. | RFC defaults |
| `denylist` | [Denylist](#denylist) | Peer / tunnel denylist behaviour. | disabled |
| `challenge-required` | bool | LNS-only: reject SCCRQ without a Challenge AVP. | `false` |
| `proxy-lcp-mode` | string | LNS-only: `forward` (re-play proxy-LCP) or `renegotiate`. | `forward` |
| `max-attempts-per-subscriber` | int | LAC-only: number of LNS candidates to try before giving up. | `4` |
| `ppp-framing` | string | PPP framing for data packets on this session: `hdlc` (Address+Control prefix present — default, matches every major LNS / pppd-based LAC) or `compressed` (ACFC, no prefix). Resolved once at session-create, the dataplane reads a single per-session byte offset on each packet with no branch on payload content. | `hdlc` |

### Retransmit

| Field | Type | Description | Default |
|-------|------|-------------|---------|
| `max-retries-not-established` | int | Retries before tunnel-setup gives up. | `5` |
| `max-retries-established` | int | Retries on established tunnel before declaring dead. | `5` |
| `initial-timeout` | duration | First retransmit timer. | `1s` |
| `max-timeout` | duration | Cap for exponential back-off. | `8s` |

### Denylist

| Field | Type | Description |
|-------|------|-------------|
| `peer-ttl` | duration | How long a denylisted peer is excluded. |
| `tunnel-ttl` | duration | How long a denylisted tunnel-spec is excluded. |
| `triggers` | [string] | CDN result codes that denylist a tunnel (`02`, `04`, `05`, `06`, `10`). |

## `l2tp.peer-policies`

LNS-only: authorize an inbound LAC by Host Name AVP and bind it to a
profile + shared secret for Challenge-AVP auth. Keyed by an arbitrary
policy name; the `hostname` field carries the actual Host Name AVP
value to match.

| Field | Type | Description |
|-------|------|-------------|
| `hostname` | string | LAC Host Name AVP value to match. |
| `secret` | string | Shared secret for Challenge AVP (when the profile or this peer requires it). |
| `profile` | string | Name of the `l2tp.profiles` entry to apply to this peer. |
| `ppp-framing` | string | Override the profile's `ppp-framing` for sessions originating from this LAC. Resolution order is per-peer-policy → profile → `hdlc`. |

## `subscriber-groups.groups.<name>.l2tp`

Binds a subscriber group to an L2TP profile.

| Field | Type | Description |
|-------|------|-------------|
| `profile` | string | Name of the `l2tp.profiles` entry. |

A LAC group declares `access-types: [lac]` on each `vlans` entry, like any
other subscriber-facing protocol. The AAA policy attached to the group maps the
subscriber to Tunnel-* attributes (local DB by `agent-remote-id` /
username, or RADIUS Access-Accept).

An LNS group is the one case that declares `access-types: [lns]` at group level
and must not declare `vlans` at all: LNS subscribers arrive inside an L2TP
tunnel, not over an SVLAN. The group's `default-service-group` selects the
loopback used as unnumbered for per-session vnet interfaces, so it must point at
a service group carrying an `unnumbered` field. Any other placement is rejected
at load: group-level `access-types` is valid only for LNS-only groups, and every
other protocol declares `access-types` per VLAN range.

## AAA contract (RFC 2868)

The LAC reads these attributes from the AAA reply to pick a tunnel:

| Attribute | Required | Notes |
|-----------|----------|-------|
| `Tunnel-Type` | yes | Must be `L2TP`. |
| `Tunnel-Medium-Type` | yes | Must be `IPv4`. |
| `Tunnel-Server-Endpoint` | yes | LNS IPv4. |
| `Tunnel-Password` | recommended | Shared secret. Falls back to pool `secret`. |
| `Tunnel-Client-Endpoint` | optional | Local source IPv4. Overrides pool `source-ipv4`. |
| `Tunnel-Preference` | optional | Lower wins when multiple candidates returned. |
| `Tunnel-Assignment-ID` | optional | Logical tunnel grouping. |

Attributes can be tagged (e.g. `tunnel.server-endpoint:1`,
`tunnel.server-endpoint:2`) to return multiple candidates in one
Access-Accept; the LAC tries them in `Tunnel-Preference` order and
denylists failures per the profile's `denylist` block.

## LAC example

```yaml
subscriber-groups:
  groups:
    pppoe-lac:
      vlan-tpid: dot1q
      vlans:
        - svlan: "200-210"
          cvlan: any
          access-types: [lac]
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
    # LAC mode: osvbng does not validate the subscriber. The LNS authenticates
    # CHAP via the proxy-auth AVPs forwarded in ICCN. The local-auth entry
    # exists only to return the Tunnel-* attributes that pick the LNS.
    - name: pppoe-policy
      type: ppp
      format: $agent-remote-id$
      authenticate: false
      max_concurrent_sessions: 1
```

## LNS example

No `tunnel-pools` block: the LNS does not originate tunnels. The group carries
group-level `access-types` and no `vlans`, and `default-service-group` supplies
the unnumbered loopback for per-session interfaces.

```yaml
service-groups:
  lns-default:
    unnumbered: loop100

subscriber-groups:
  groups:
    lns:
      access-types: [lns]
      ipv4-profile: default
      ipv6-profile: default-v6
      aaa-policy: lns-policy
      default-service-group: lns-default
      l2tp:
        profile: LNS_DEFAULT

l2tp:
  profiles:
    LNS_DEFAULT:
      receive-window-size: 16
      hello-interval: 60s
      challenge-required: false
      proxy-lcp-mode: forward
  peer-policies:
    xl2tpd:
      hostname: "lac"
      secret: "shared"
      profile: LNS_DEFAULT

aaa:
  auth_provider: local
  nas_identifier: osvbng
  policy:
    # LNS mode: osvbng terminates PPP, so it authenticates the subscriber
    # itself unless the LAC forwarded proxy-auth AVPs.
    - name: lns-policy
      type: ppp
      authenticate: true
      max_concurrent_sessions: 1
```

## Show commands

```
$ osvbngcli show l2tp tunnels        # tunnel-level: local/peer IPs, state, session count
$ osvbngcli show subscriber sessions # subscriber-level; LAC rows have State=tunneled
                                     # plus an L2TP sub-object with tunnel/session IDs
```

The subscriber view embeds an `L2TP` object only when the session is
tunneled (LAC mode), so IPoE and non-LAC PPPoE subscribers render the
same JSON shape they always have. Per-subscriber L2TP details appear
alongside the existing PPPoE fields rather than as a separate listing.

## See also

- [LAC deployment example](../examples/l2tp-lac.md)
- [LNS deployment example](../examples/l2tp-lns.md)
- [AAA configuration](aaa.md)
- [Subscriber groups](subscriber-groups.md)
- RFC 2661, RFC 2868, RFC 3437
