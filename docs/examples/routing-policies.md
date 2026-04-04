# Routing Policy Examples

Real-world routing policy examples adapted from production ISP configurations. All ASNs, prefixes, and communities use documentation/private ranges — replace them with your own values.

In these examples, `64500` is the local ASN.

## Bogon Filtering

Reject RFC1918, documentation, multicast, and other non-routable prefixes on inbound eBGP sessions. Every ISP should have bogon filters on all external peers.

```yaml
routing-policies:
  prefix-sets:
    BOGONS-V4:
      - prefix: "0.0.0.0/8"
        le: 32
        action: permit
      - prefix: "10.0.0.0/8"
        action: permit
      - prefix: "100.64.0.0/10"
        action: permit
      - prefix: "127.0.0.0/8"
        action: permit
      - prefix: "169.254.0.0/16"
        action: permit
      - prefix: "172.16.0.0/12"
        action: permit
      - prefix: "192.0.0.0/24"
        action: permit
      - prefix: "192.0.2.0/24"
        action: permit
      - prefix: "192.168.0.0/16"
        action: permit
      - prefix: "198.18.0.0/15"
        action: permit
      - prefix: "198.51.100.0/24"
        action: permit
      - prefix: "203.0.113.0/24"
        action: permit
      - prefix: "224.0.0.0/4"
        action: permit
      - prefix: "240.0.0.0/4"
        action: permit
    TOO-SPECIFIC-V4:
      - prefix: "0.0.0.0/0"
        ge: 25
        action: permit

  prefix-sets-v6:
    BOGONS-V6:
      - prefix: "::/8"
        action: permit
      - prefix: "::1/128"
        action: permit
      - prefix: "::ffff:0:0/96"
        le: 128
        action: permit
      - prefix: "100::/64"
        le: 128
        action: permit
      - prefix: "2001::/32"
        le: 128
        action: permit
      - prefix: "2001:2::/48"
        le: 128
        action: permit
      - prefix: "2001:10::/28"
        le: 128
        action: permit
      - prefix: "2001:db8::/32"
        le: 128
        action: permit
      - prefix: "fc00::/7"
        le: 128
        action: permit
      - prefix: "fe80::/10"
        le: 128
        action: permit
      - prefix: "fec0::/10"
        le: 128
        action: permit
      - prefix: "ff00::/8"
        le: 128
        action: permit
    TOO-SPECIFIC-V6:
      - prefix: "::/0"
        ge: 49
        action: permit

  route-policies:
    REJECT-BOGONS-V4:
      - sequence: 10
        action: deny
        match:
          prefix-set: BOGONS-V4
      - sequence: 20
        action: deny
        match:
          prefix-set: TOO-SPECIFIC-V4
      - sequence: 1000
        action: permit

    REJECT-BOGONS-V6:
      - sequence: 10
        action: deny
        match:
          prefix-set-v6: BOGONS-V6
      - sequence: 20
        action: deny
        match:
          prefix-set-v6: TOO-SPECIFIC-V6
      - sequence: 1000
        action: permit
```

## Remote Triggered Black Hole (RTBH)

RTBH allows an operator to signal that a prefix should be null-routed network-wide. A /32 host route tagged with a blackhole community is advertised to peers, which then drop traffic to that destination.

```yaml
routing-policies:
  community-sets:
    BLACKHOLE:
      - "64500:666"

  route-policies:
    # Apply inbound on eBGP peers — drop any route tagged with blackhole community
    BLACKHOLE-FILTER:
      - sequence: 10
        action: deny
        match:
          community-set: BLACKHOLE
      - sequence: 1000
        action: permit

    # Apply to iBGP — accept blackhole routes and set next-hop to null
    BLACKHOLE-ACCEPT:
      - sequence: 10
        action: permit
        match:
          community-set: BLACKHOLE
        set:
          next-hop-ipv4: "192.0.2.1"
      - sequence: 1000
        action: permit

    # Tag a /32 with the blackhole community for advertisement
    TAG-BLACKHOLE:
      - sequence: 10
        action: permit
        set:
          community: "64500:666"
```

To trigger a blackhole, advertise a /32 tagged via the route-policy and add a null route so BGP can originate it:

```yaml
routing-policies:
  route-policies:
    TAG-BLACKHOLE:
      - sequence: 10
        action: permit
        set:
          community: "64500:666"

protocols:
  static:
    ipv4:
      - destination: "203.0.113.1/32"
        device: Null0
  bgp:
    ipv4-unicast:
      networks:
        203.0.113.1/32:
          route-policy: TAG-BLACKHOLE
```

## Geographic / Regional Communities

Tag routes with communities to indicate region of origin. Useful for traffic engineering and selective advertisement to peers.

```yaml
routing-policies:
  community-sets:
    REGION-NORTH:
      - "64500:1000"
    REGION-SOUTH:
      - "64500:2000"
    REGION-EAST:
      - "64500:3000"
    REGION-WEST:
      - "64500:4000"

  large-community-sets:
    # Large communities allow encoding more context: ASN:function:parameter
    # Function 1 = region, Function 2 = peer type, Function 3 = site ID
    LC-REGION-NORTH:
      - "64500:1:1000"
    LC-REGION-SOUTH:
      - "64500:1:2000"

  route-policies:
    # Tag redistributed routes with the local region
    TAG-REGION-NORTH:
      - sequence: 10
        action: permit
        set:
          community: "64500:1000"
          community-additive: true
          large-community: "64500:1:1000"
          large-community-additive: true

    # Only advertise routes from a specific region to a peer
    EXPORT-NORTH-ONLY:
      - sequence: 10
        action: permit
        match:
          community-set: REGION-NORTH
      - sequence: 1000
        action: deny
```

## Peering vs Transit Local-Preference

Differentiate path preference by peer type. Higher local-preference wins in BGP best-path selection. Typical hierarchy: customer > private peering > IXP peering > transit.

```yaml
routing-policies:
  route-policies:
    # Transit upstreams — lowest preference (default BGP is 100)
    TRANSIT-IN-V4:
      - sequence: 10
        action: deny
        match:
          prefix-set: BOGONS-V4
      - sequence: 100
        action: permit
        set:
          local-preference: 100
          community: "64500:950"
          community-additive: true

    # IXP peers — medium preference
    IXP-PEER-IN-V4:
      - sequence: 10
        action: deny
        match:
          prefix-set: BOGONS-V4
      - sequence: 100
        action: permit
        set:
          local-preference: 140
          community: "64500:930"
          community-additive: true

    # Private peering (direct interconnect) — higher preference
    PNI-PEER-IN-V4:
      - sequence: 10
        action: deny
        match:
          prefix-set: BOGONS-V4
      - sequence: 100
        action: permit
        set:
          local-preference: 150
          community: "64500:920"
          community-additive: true

    # Customer routes — highest preference
    CUSTOMER-IN-V4:
      - sequence: 10
        action: deny
        match:
          prefix-set: BOGONS-V4
      - sequence: 100
        action: permit
        set:
          local-preference: 200
          community: "64500:960"
          community-additive: true
```

## AS-Path Filtering

Reject routes that contain private ASNs in the path. These should never appear in the global routing table.

```yaml
routing-policies:
  as-path-sets:
    PRIVATE-ASN:
      - regex: "_6[4-5][0-9][0-9][0-9]_"
        action: permit
      - regex: "_6553[0-5]_"
        action: permit
      - regex: "_42[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]_"
        action: permit

  route-policies:
    REJECT-PRIVATE-ASN:
      - sequence: 10
        action: deny
        match:
          as-path-set: PRIVATE-ASN
      - sequence: 1000
        action: permit
```

## Outbound Scrubbing

Strip internal communities and prepend AS-path before advertising to external peers. Prevents internal traffic engineering signals from leaking.

```yaml
routing-policies:
  prefix-sets:
    OWN-PREFIXES-V4:
      - prefix: "192.0.2.0/24"
        action: permit
      - prefix: "198.51.100.0/24"
        action: permit
      - prefix: "203.0.113.0/24"
        action: permit

  community-sets:
    INTERNAL-COMMS:
      - "64500:920"
      - "64500:930"
      - "64500:950"
      - "64500:960"
    NO-EXPORT-TO-PEERS:
      - "64500:640"

  as-path-sets:
    OWN-ORIGIN:
      - regex: "^$"
        action: permit

  route-policies:
    PEER-OUT-V4:
      # Block routes tagged no-export
      - sequence: 10
        action: deny
        match:
          community-set: NO-EXPORT-TO-PEERS
      # Only advertise own prefixes originated locally
      - sequence: 20
        action: permit
        match:
          prefix-set: OWN-PREFIXES-V4
          as-path-set: OWN-ORIGIN
        set:
          community-delete: INTERNAL-COMMS
      # Deny everything else
      - sequence: 1000
        action: deny

    # Transit export — advertise own + customer routes, prepend on backup transit
    TRANSIT-OUT-V4:
      - sequence: 10
        action: deny
        match:
          community-set: NO-EXPORT-TO-PEERS
      - sequence: 20
        action: permit
        match:
          prefix-set: OWN-PREFIXES-V4
        set:
          community-delete: INTERNAL-COMMS
      - sequence: 1000
        action: deny

    # Backup transit — same as primary but with AS-path prepend for depref
    BACKUP-TRANSIT-OUT-V4:
      - sequence: 10
        action: deny
        match:
          community-set: NO-EXPORT-TO-PEERS
      - sequence: 20
        action: permit
        match:
          prefix-set: OWN-PREFIXES-V4
        set:
          as-path-prepend: "64500 64500"
          community-delete: INTERNAL-COMMS
      - sequence: 1000
        action: deny
```

## Customer Prefix Filtering

Accept only a customer's registered prefixes. Each customer gets a dedicated prefix-set.

```yaml
routing-policies:
  prefix-sets:
    # Per-customer prefix sets — populated from IRR/RPKI or manual registration
    CUST-ACME-V4:
      - prefix: "192.0.2.0/24"
        le: 24
        action: permit
    CUST-ACME-V6:
      - prefix: "2001:db8:abcd::/48"
        le: 48
        action: permit

  route-policies:
    CUST-ACME-IN-V4:
      - sequence: 10
        action: deny
        match:
          prefix-set: BOGONS-V4
      - sequence: 20
        action: permit
        match:
          prefix-set: CUST-ACME-V4
        set:
          local-preference: 200
          community: "64500:960"
          community-additive: true
          large-community: "64500:44:12345"
          large-community-additive: true
      - sequence: 1000
        action: deny
```

## Redistribute Filtering

Control which connected routes are redistributed into BGP. Prevents infrastructure /30 links and loopbacks from leaking into the BGP table.

```yaml
routing-policies:
  prefix-sets:
    REDIST-ALLOWED-V4:
      - prefix: "192.0.2.0/24"
        le: 24
        action: permit
      - prefix: "198.51.100.0/24"
        le: 24
        action: permit

  route-policies:
    REDIST-CONNECTED-V4:
      - sequence: 10
        action: permit
        match:
          prefix-set: REDIST-ALLOWED-V4
      - sequence: 1000
        action: deny
```

```yaml
protocols:
  bgp:
    ipv4-unicast:
      redistribute:
        connected: true
        route-policy: REDIST-CONNECTED-V4
  ospf:
    redistribute:
      connected: true
      route-policy: REDIST-CONNECTED-V4
```

## Full Peering Policy (Combined)

A complete inbound + outbound policy for an IXP peer, combining bogon filtering, blackhole rejection, community tagging, and outbound scrubbing.

```yaml
routing-policies:
  prefix-sets:
    BOGONS-V4:
      - prefix: "10.0.0.0/8"
        action: permit
      - prefix: "172.16.0.0/12"
        action: permit
      - prefix: "192.168.0.0/16"
        action: permit
    TOO-SPECIFIC-V4:
      - prefix: "0.0.0.0/0"
        ge: 25
        action: permit
    OWN-PREFIXES-V4:
      - prefix: "192.0.2.0/24"
        action: permit
      - prefix: "198.51.100.0/24"
        action: permit

  community-sets:
    BLACKHOLE:
      - "64500:666"
    IXP-TAGGED:
      - "64500:930"
    INTERNAL-COMMS:
      - "64500:920"
      - "64500:930"
      - "64500:950"
      - "64500:960"
    NO-EXPORT-IXP:
      - "64500:6661"

  as-path-sets:
    PRIVATE-ASN:
      - regex: "_6[4-5][0-9][0-9][0-9]_"
        action: permit
    OWN-ORIGIN:
      - regex: "^$"
        action: permit

  route-policies:
    IXP-PEER-IN-V4:
      - sequence: 10
        action: deny
        match:
          prefix-set: BOGONS-V4
      - sequence: 20
        action: deny
        match:
          prefix-set: TOO-SPECIFIC-V4
      - sequence: 30
        action: deny
        match:
          community-set: BLACKHOLE
      - sequence: 40
        action: deny
        match:
          as-path-set: PRIVATE-ASN
      - sequence: 100
        action: permit
        set:
          local-preference: 140
          community: "64500:930"
          community-additive: true

    IXP-PEER-OUT-V4:
      - sequence: 10
        action: deny
        match:
          prefix-set: BOGONS-V4
      - sequence: 20
        action: deny
        match:
          prefix-set: TOO-SPECIFIC-V4
      - sequence: 30
        action: deny
        match:
          community-set: NO-EXPORT-IXP
      - sequence: 100
        action: permit
        match:
          prefix-set: OWN-PREFIXES-V4
          as-path-set: OWN-ORIGIN
        set:
          community-delete: INTERNAL-COMMS
      - sequence: 1000
        action: deny
```

```yaml
protocols:
  bgp:
    peer-groups:
      IXP-PEERS:
        remote-as: 64501
        ipv4-unicast:
          send-community: both
          route-policy-in: IXP-PEER-IN-V4
          route-policy-out: IXP-PEER-OUT-V4
    neighbors:
      192.0.2.100:
        peer-group: IXP-PEERS
        description: "AS64501 - Example Peer at IXP-A"
      192.0.2.101:
        peer-group: IXP-PEERS
        description: "AS64501 - Example Peer at IXP-B"
```
