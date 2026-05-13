# L2TP

L2TPv2 (RFC 2661) configuration. osvbng supports the **LAC** role — terminating
PPPoE locally and tunnelling the subscriber's PPP frames over L2TP to a remote
LNS. The LNS role is on the roadmap.

The block configures three things: per-LNS endpoint pools (`tunnel-pools`),
behavioural profiles (`profiles`), and authorization for inbound LAC peers
(`peer-policies`, LNS-only).

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

## `subscriber-groups.groups.<name>.l2tp`

Binds a subscriber group to an L2TP profile.

| Field | Type | Description |
|-------|------|-------------|
| `profile` | string | Name of the `l2tp.profiles` entry. |

The group's `access-type` must be `lac`. The AAA policy attached to the
group is consulted to map the subscriber to Tunnel-* attributes (either
through local-DB attributes by `agent-remote-id` / username, or via a
RADIUS Access-Accept).

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

## Example

```yaml
subscriber-groups:
  groups:
    pppoe-lac:
      access-type: lac
      vlan-tpid: dot1q
      vlans:
        - svlan: "200-210"
          cvlan: any
          interface: loop100
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
- [AAA configuration](aaa.md)
- [Subscriber groups](subscriber-groups.md)
- RFC 2661, RFC 2868, RFC 3437
