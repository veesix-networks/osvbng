# IPoE + PPPoE BNG

A combined QinQ deployment serving both IPoE and PPPoE subscribers on the same BNG. Each access type uses a different S-VLAN range and AAA policy, but shares the same address pools and routing infrastructure.

This is the most common deployment model for ISPs migrating from PPPoE to IPoE — existing PPPoE subscribers continue to work while new deployments use IPoE. Both access types arrive on the same physical interface, differentiated by S-VLAN range.

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
    ipoe-residential:
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
      aaa-policy: ipoe-policy
    pppoe-residential:
      access-type: pppoe
      ipv4-profile: residential-v4
      ipv6-profile: residential-v6
      vlans:
        - svlan: "200-299"
          cvlan: any
          interface: loop100
      aaa-policy: pppoe-policy

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
    - name: ipoe-policy
      format: $remote-id$
      max_concurrent_sessions: 1
    - name: pppoe-policy
      type: ppp
      format: $agent-remote-id$
      authenticate: true
      max_concurrent_sessions: 1

plugins:
  northbound.api:
    enabled: true
    listen_address: :8080
  subscriber.auth.local:
    allow_all: false
    database_path: /var/lib/osvbng/subscribers.db

logging:
  format: text
  level: info

```

## Key Points

- **Shared address pools** — both IPoE and PPPoE subscribers draw from the same `residential-v4` and `residential-v6` profiles
- **Separate QinQ VLAN ranges** — IPoE on S-VLAN 100-199, PPPoE on S-VLAN 200-299
- **Separate AAA policies** — IPoE uses `$remote-id$` for identification, PPPoE uses `$agent-remote-id$` with `authenticate: true` to validate CHAP/PAP credentials
- **Single access interface** — both access types share `eth1` with `bng_mode: access`. osvbng demultiplexes by protocol (DHCP discovery vs PPPoE discovery) and VLAN
- **BGP pool advertisement** is only on the IPoE group in this example — enable it on both if PPPoE subscribers also need their pools advertised to the core
