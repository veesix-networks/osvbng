# IPoE BNG

A basic IPoE (IP over Ethernet) BNG deployment with dual-stack subscribers over QinQ (S-VLAN + C-VLAN), local authentication, and OSPF/BGP routing.

Subscribers connect via QinQ double-tagged access ports. Each subscriber receives an IPv4 address via DHCP and IPv6 via DHCPv6 (IANA + prefix delegation).

## Configuration

```yaml
routing-policies:
  route-policies:
    POOL-EXPORT:
      - sequence: 10
        action: permit
        set:
          community: "64500:960"
          community-additive: true

subscriber-groups:
  groups:
    residential:
      access-type: ipoe
      ipv4-profile: residential-v4
      ipv6-profile: residential-v6
      vlans:
        - svlan: "100-199"
          cvlan: any
          interface: loop100
      bgp:
        enabled: true
        advertise-pools: true
        network-route-policy: POOL-EXPORT
      aaa-policy: default-policy

ipv4-profiles:
  residential-v4:
    gateway: 10.255.0.1
    dns:
      - 8.8.8.8
      - 8.8.4.4
    pools:
      - name: subscriber-pool
        network: 10.255.0.0/16
        priority: 1
    dhcp:
      lease-time: 3600

ipv6-profiles:
  residential-v6:
    iana-pools:
      - name: wan-link-pool
        network: 2001:db8:0:1::/64
        range_start: 2001:db8:0:1::1000
        range_end: 2001:db8:0:1::ffff
        gateway: 2001:db8:0:1::1
        preferred_time: 3600
        valid_time: 7200
    pd-pools:
      - name: subscriber-pd-pool
        network: 2001:db8:100::/40
        prefix_length: 56
        preferred_time: 3600
        valid_time: 7200
    dns:
      - 2001:4860:4860::8888
      - 2001:4860:4860::8844

dhcp:
  provider: local

dhcpv6:
  provider: local
  dns_servers:
    - 2001:4860:4860::8888
    - 2001:4860:4860::8844
  ra:
    router_lifetime: 1800
    max_interval: 600
    min_interval: 200

interfaces:
  loop0:
    description: Control Plane Loopback
    enabled: true
    address:
      ipv4:
        - 10.254.0.1/32
    lcp: true
  eth1:
    description: Access Interface
    enabled: true
    bng_mode: access
  eth2:
    description: Core Uplink
    enabled: true
    lcp: true
    address:
      ipv4:
        - 10.0.0.1/30
      ipv6:
        - 2001:db8:c0:e::1/64
  loop100:
    description: Subscriber Gateway
    enabled: true
    address:
      ipv4:
        - 10.255.0.1/32
      ipv6:
        - 2001:db8:0:1::1/128
    lcp: true

protocols:
  bgp:
    asn: 64500
    router-id: 10.254.0.1
    neighbors:
      10.254.0.2:
        remote-as: 64500
        peer: loop0
    ipv4-unicast: {}
  ospf:
    enabled: true
    router-id: 10.254.0.1
    log-adjacency-changes: true
    areas:
      "0.0.0.0":
        interfaces:
          eth2:
            network: point-to-point
          loop0:
            passive: true
  ospf6:
    enabled: true
    router-id: 10.254.0.1
    log-adjacency-changes: true
    areas:
      "0.0.0.0":
        interfaces:
          eth2:
            network: point-to-point

aaa:
  auth_provider: local
  nas_identifier: osvbng
  policy:
    - name: default-policy
      format: $remote-id$
      max_concurrent_sessions: 1

plugins:
  northbound.api:
    enabled: true
    listen_address: :8080
  subscriber.auth.local:
    allow_all: true
    database_path: /var/lib/osvbng/subscribers.db

logging:
  format: text
  level: info

```

## Key Points

- **Access interface** (`eth1`) is set to `bng_mode: access` — this is where subscriber-facing VLANs arrive
- **Subscriber gateway** (`loop100`) is the unnumbered interface for subscriber sessions — its address is the default gateway
- **QinQ VLAN matching** — `svlan: "100-199"` matches the outer S-VLAN range; `cvlan: any` accepts any inner C-VLAN
- **BGP pool advertisement** (`advertise-pools: true`) automatically creates BGP `network` statements for the subscriber pools so the core network can route to subscribers. The optional `network-route-policy` controls how these prefixes are advertised (e.g. community tagging). If `advertise-pools` is disabled, you must manually configure the `network` statements under `protocols.bgp.ipv4-unicast.networks`
- **OSPF** provides IGP reachability between the BNG loopback and the core router for iBGP next-hop resolution
- **Local auth** with `allow_all: true` accepts any subscriber without a database entry — useful for lab/testing, disable in production
