# PPPoE BNG

A PPPoE (Point-to-Point Protocol over Ethernet) BNG deployment with dual-stack subscribers over QinQ (S-VLAN + C-VLAN), local authentication with per-subscriber credentials, and OSPF/BGP routing.

Subscribers connect via PPPoE discovery on QinQ double-tagged access ports. Each session is authenticated via the AAA policy before an IP address is assigned.

## Configuration

```yaml
subscriber-groups:
  groups:
    residential:
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

## Key Differences from IPoE

- **`access-type: pppoe`** — subscribers establish PPPoE sessions instead of raw DHCP
- **AAA policy `type: ppp`** — identifies this as a PPP-based session
- **`authenticate: true`** — every PPPoE session must pass authentication before IP assignment
- **`format: $agent-remote-id$`** — subscriber identity is derived from the agent remote ID, a value inserted by the access network provider
- **`authenticate: true`** — PPPoE CHAP/PAP credentials are validated against the auth provider. If set to `false`, CHAP/PAP automatically passes regardless of the password provided
- **`allow_all: false`** on the local auth plugin — subscribers must have a database entry to authenticate

