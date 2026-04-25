# VRF-Lite

A VRF-lite deployment with two customer VRFs sharing a BNG, isolated via per-VRF core sub-interfaces and per-VRF iBGP sessions to a core router. No MPLS required.

Subscribers are assigned to a VRF via their service group. The `default-service-group` on the subscriber group sets the service group, which carries the `vrf:` field and the pool to allocate from.

## Topology

```
subscribers -- bng1 (osvbng) -- corerouter1 (FRR)
                  eth2.100 (DEFAULT VRF)
                  eth2.200 (CUSTOMER-A VRF)
```

## Configuration

```yaml
vrfs:
  CUSTOMER-A:
    description: "VRF-lite customer A"
    address-families:
      ipv4-unicast: {}

service-groups:
  customer-a:
    vrf: CUSTOMER-A
    unnumbered: loop101
    pool: customer-a-pool

subscriber-groups:
  groups:
    default-ipoe:
      access-type: ipoe
      vlan-tpid: dot1q
      ipv4-profile: default
      vlans:
        - svlan: "100-199"
          cvlan: any
          interface: loop100
      aaa-policy: default-policy

    customer-a-ipoe:
      access-type: ipoe
      vlan-tpid: dot1q
      ipv4-profile: default
      default-service-group: customer-a
      vlans:
        - svlan: "200-299"
          cvlan: any
          interface: loop101
      aaa-policy: default-policy

ipv4-profiles:
  default:
    gateway: 10.255.0.1
    pools:
      - name: subscriber-pool
        network: 10.255.0.0/16
        priority: 1
      - name: customer-a-pool
        network: 192.168.123.0/24
        vrf: CUSTOMER-A
        priority: 10

interfaces:
  loop0:   {address: {ipv4: [10.254.0.1/32]}}
  loop100: {address: {ipv4: [10.255.0.1/32]}}
  loop101:
    address: {ipv4: [192.168.123.1/32]}
    vrf: CUSTOMER-A
  eth1: {bng_mode: access}
  eth2:
    enabled: true
    subinterfaces:
      - id: 100
        vlan: 100
        enabled: true
        address: {ipv4: [10.0.100.1/30]}
      - id: 200
        vlan: 200
        enabled: true
        vrf: CUSTOMER-A
        address: {ipv4: [10.0.200.1/30]}

protocols:
  bgp:
    asn: 65000
    router-id: 10.254.0.1
    ipv4-unicast: {}
    neighbors:
      10.0.100.2:
        remote-as: 65000
        ipv4-unicast: {}
    vrf:
      CUSTOMER-A:
        ipv4-unicast:
          redistribute: {connected: true}
        neighbors:
          10.0.200.2:
            remote-as: 65000
            ipv4-unicast: {}
```

## Key Points

- `default-service-group: customer-a` on the subscriber group is what binds the session into the CUSTOMER-A VRF. Without it, sessions use the default VRF regardless of the pool's `vrf:` field.
- Each VRF has its own core sub-interface (`eth2.100` and `eth2.200`) and BGP session. There is no global `redistribute connected`; each VRF's redistribute is scoped to that VRF's address-family stanza.
- Pool `vrf: CUSTOMER-A` on `customer-a-pool` ensures DHCP allocation only draws from that pool for CUSTOMER-A sessions. It does not set the session VRF.
