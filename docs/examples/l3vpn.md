# MPLS L3VPN

An MPLS L3VPN (RFC 4364) deployment with VPNv4 iBGP between two PE routers and LDP transport labels. Subscribers in the CUSTOMER-A VRF reach destinations on the remote PE via a two-label MPLS stack (transport outer, VPN inner).

## Topology

```
subscribers -- bng1 (PE1, osvbng) -- p1 (P router, FRR) -- corerouter1 (PE2, FRR)
```

- bng1: osvbng PE with LDP, OSPF, and VPNv4 iBGP to corerouter1
- p1: transit P router with LDP and OSPF, no BGP
- corerouter1: FRR PE with VPN import/export and VRF-scoped destinations

## bng1 Configuration

```yaml
vrfs:
  CUSTOMER-A:
    description: "MPLS L3VPN customer A"
    rd: "65000:100"
    import-route-targets: ["65000:100"]
    export-route-targets: ["65000:100"]
    address-families:
      ipv4-unicast: {}

service-groups:
  customer-a:
    vrf: CUSTOMER-A
    unnumbered: loop101
    pool: customer-a-pool

subscriber-groups:
  groups:
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

interfaces:
  loop0:   {address: {ipv4: [10.254.0.1/32]}}
  loop100: {address: {ipv4: [10.255.0.1/32]}}
  loop101:
    address: {ipv4: [192.168.123.1/32]}
    vrf: CUSTOMER-A
  eth1: {bng_mode: access}
  eth2:
    description: Core Link (p1)
    address: {ipv4: [10.0.0.1/30]}

protocols:
  bgp:
    asn: 65000
    router-id: 10.254.0.1
    neighbors:
      10.254.0.2:
        remote-as: 65000
        peer: loop0
    ipv4-vpn:
      neighbors:
        10.254.0.2:
          send-community: both
    vrf:
      CUSTOMER-A:
        ipv4-unicast:
          redistribute: {connected: true}
          label-vpn: auto
          export-vpn: true
          import-vpn: true
  ospf:
    enabled: true
    router-id: 10.254.0.1
    areas:
      "0.0.0.0":
        interfaces:
          eth2: {network: point-to-point}
          loop0: {passive: true}
  mpls: {enabled: true}
  ldp:
    enabled: true
    router-id: 10.254.0.1
    address-families:
      ipv4:
        transport-address: 10.254.0.1
```

## Key Points

- `rd`, `import-route-targets`, and `export-route-targets` on the VRF config enable L3VPN behaviour. Without `rd`, the VRF is VRF-lite only.
- `label-vpn: auto` on the per-VRF BGP stanza tells FRR to allocate a VPN label automatically for exported prefixes.
- `export-vpn: true` and `import-vpn: true` enable VPNv4 cross-VRF advertisement.
- OSPF provides loopback reachability for the iBGP session (`update-source loop0` on the neighbor).
- With a P router between the two PEs, the P router is the penultimate hop, so the full two-label stack (transport outer, VPN inner) appears on egress from PE1.
- `default-service-group: customer-a` on the subscriber group is still required to bind subscribers into the CUSTOMER-A VRF. It is not inferred from the pool or the VRF config.
