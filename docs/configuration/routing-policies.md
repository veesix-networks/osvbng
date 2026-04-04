# Routing Policies

Routing policies define control-plane filtering and route manipulation objects that are applied to BGP neighbors, redistribute statements, and network advertisements. These translate to FRR prefix-lists, community-lists, AS-path access-lists, and route-maps.

`routing-policies` is a top-level configuration section because policies are cross-cutting — they can be referenced by BGP, OSPF, and IS-IS.

## Prefix Sets

IPv4 and IPv6 prefix sets are defined separately under `prefix-sets` and `prefix-sets-v6`. Each translates to FRR `ip prefix-list` and `ipv6 prefix-list` respectively. The handler validates that only the correct address family is configured in each section.

Each entry:

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `sequence` | int | Explicit sequence number (auto-generated in steps of 10 if omitted) | `15` |
| `prefix` | string | CIDR prefix to match | `10.0.0.0/8` |
| `le` | int | Match prefix lengths less than or equal to this value | `24` |
| `ge` | int | Match prefix lengths greater than or equal to this value | `16` |
| `action` | string | `permit` or `deny` | `permit` |

```yaml
routing-policies:
  prefix-sets:
    CUSTOMER-V4:
      - prefix: "10.0.0.0/8"
        le: 24
        action: permit
      - prefix: "172.16.0.0/12"
        action: permit
  prefix-sets-v6:
    CUSTOMER-V6:
      - prefix: "2001:db8::/32"
        le: 48
        action: permit
```

## Community Sets

Define BGP standard community match lists. Translates to FRR `bgp community-list standard`.

Each community set is a list of community strings in `AA:NN` format or well-known names: `no-export`, `no-advertise`, `no-peer`, `blackhole`, `local-AS`, `internet`.

```yaml
routing-policies:
  community-sets:
    BLACKHOLE:
      - "65000:666"
    CUSTOMERS:
      - "65000:100"
      - "65000:200"
```

## Extended Community Sets

Define BGP extended community match lists. Translates to FRR `bgp extcommunity-list standard`.

Each member must start with a type keyword (`rt` or `soo`) followed by a value in one of these formats: `AA:NN`, `AS4:NN`, or `A.B.C.D:NN`.

```yaml
routing-policies:
  ext-community-sets:
    RT-VPN-A:
      - "rt 65000:100"
      - "rt 65000:200"
    SOO-SITE1:
      - "soo 65000:1"
```

## Large Community Sets

Define BGP large community match lists. Translates to FRR `bgp large-community-list standard`.

Each member uses the `GLOBAL:LOCAL1:LOCAL2` format (three colon-separated integers).

```yaml
routing-policies:
  large-community-sets:
    LC-BLACKHOLE:
      - "65000:666:0"
    LC-CUSTOMERS:
      - "65000:100:1"
```

## AS-Path Sets

Define BGP AS-path regex match lists. Translates to FRR `bgp as-path access-list`.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `regex` | string | Regular expression to match against the AS path | `^$` |
| `action` | string | `permit` or `deny` | `permit` |

```yaml
routing-policies:
  as-path-sets:
    OWN-AS:
      - regex: "^$"
        action: permit
    TRANSIT:
      - regex: ".*"
        action: permit
```

## Route Policies

Define route manipulation policies. Translates to FRR `route-map` entries. A route-policy is a directionless ordered list of match/set rules — direction is determined by where the policy is attached (BGP neighbor `route-policy-in`/`route-policy-out`, redistribute, etc.).

Each entry:

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `sequence` | int | Order in which entries are evaluated (must be unique, > 0) | `10` |
| `action` | string | `permit` or `deny` | `permit` |
| `match` | object | Match conditions (all optional) | |
| `set` | object | Set actions applied when matched (all optional) | |
| `call` | string | Call another route-policy by name | `SUB-POLICY` |
| `on-match` | string | Exit policy: `next` or `goto N` | `next` |

### Match Conditions

| Field | Type | Description |
|-------|------|-------------|
| `prefix-set` | string | Match IPv4 prefixes against a named entry in `prefix-sets` |
| `prefix-set-v6` | string | Match IPv6 prefixes against a named entry in `prefix-sets-v6` |
| `community-set` | string | Match standard communities against a named community-set |
| `ext-community-set` | string | Match extended communities against a named ext-community-set |
| `large-community-set` | string | Match large communities against a named large-community-set |
| `as-path-set` | string | Match AS path against a named as-path-set |
| `metric` | int | Match route metric (MED) |
| `tag` | int | Match route tag |

### Set Actions

| Field | Type | Description |
|-------|------|-------------|
| `local-preference` | int | Set BGP local preference |
| `metric` | int | Set route metric (MED) |
| `weight` | int | Set BGP weight |
| `community` | string | Set standard community value |
| `community-additive` | bool | Append community instead of replacing |
| `community-delete` | string | Delete communities matching a named community-set |
| `large-community` | string | Set large community value |
| `large-community-additive` | bool | Append large community instead of replacing |
| `ext-community-rt` | string | Set extended community route target |
| `ext-community-soo` | string | Set extended community site-of-origin |
| `as-path-prepend` | string | Prepend AS numbers (space-separated ASNs or `last-as N`) |
| `origin` | string | Set BGP origin: `igp`, `egp`, or `incomplete` |
| `tag` | int | Set route tag |
| `next-hop-ipv4` | string | Set IPv4 next-hop address |
| `next-hop-ipv6` | string | Set IPv6 next-hop address |

### Route Policy Example

```yaml
routing-policies:
  prefix-sets:
    CUSTOMER-V4:
      - prefix: "10.0.0.0/8"
        le: 24
        action: permit

  community-sets:
    BLACKHOLE:
      - "65000:666"
    INTERNAL:
      - "65000:999"

  route-policies:
    CUSTOMER-IN:
      - sequence: 10
        action: permit
        match:
          prefix-set: CUSTOMER-V4
        set:
          local-preference: 200
          community: "65000:100"
          community-additive: true
      - sequence: 1000
        action: deny

    BLACKHOLE-IN:
      - sequence: 10
        action: permit
        match:
          community-set: BLACKHOLE
        set:
          next-hop-ipv4: "192.0.2.1"
      - sequence: 1000
        action: deny

    STRIP-INTERNAL:
      - sequence: 10
        action: permit
        set:
          community-delete: INTERNAL
```

### Attaching Route Policies

Route policies are attached to BGP neighbors, peer groups, network advertisements, and redistribute statements:

```yaml
protocols:
  bgp:
    peer-groups:
      CUSTOMERS:
        ipv4-unicast:
          route-policy-in: CUSTOMER-IN
          route-policy-out: CUSTOMER-OUT
    ipv4-unicast:
      redistribute:
        connected: true
        route-policy: REDIST-CONNECTED

  ospf:
    redistribute:
      connected: true
      route-policy: OSPF-REDIST-FILTER

  isis:
    redistribute:
      ipv4-connected: true
      route-policy: ISIS-REDIST-FILTER
```
