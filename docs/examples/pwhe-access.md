# Subscriber Termination over VXLAN Access (Pseudowire Headend)

IPoE, PPPoE, and L2TP LAC subscribers arriving over a VXLAN access
network instead of a physical port. The access operator's leaf switch
stitches the QinQ NNI into a VXLAN service; osvbng terminates the
tunnel on a **pseudowire headend** - a virtual access port that
behaves exactly like a physical interface: VLAN sub-interfaces classify
on it, subscriber groups parent on it, and every access protocol
terminates on it unchanged.

```
 Subscribers      Access leaf                osvbng
 (QinQ,           +---------+           +-------------------+
  S 100-110) ---- | VNI     | ~~VXLAN~~ | vxlan-an1         |
                  | 10101   |           |   -> pw-an1       |
                  +---------+           |      (headend)    |
                                        |  IPoE / PPPoE     |
                                        |  local sessions,  |
                                        |  or LAC -> LNS    |
                                        +-------------------+
```

The tunnel can be **static** (`dst:` pointing at the leaf's service
VTEP) or **EVPN-signaled** (`signaling: evpn`, remote VTEP discovered -
see [EVPN-VXLAN Fabric](evpn-fabric.md)). Everything below is identical
either way; the two lines that differ are marked.

## Transport and headend

```yaml
interfaces:
  eth1:
    description: Access underlay
    enabled: true
    mtu: 9000
    address:
      ipv4: [10.98.1.1/24]
  loop0:
    description: VTEP loopback
    enabled: true
    address:
      ipv4: [10.254.1.1/32]
  vxlan-an1:
    description: Access operator NNI transport
    enabled: true
    vxlan:
      src-interface: loop0
      vni: 10101
      signaling: evpn        # or omit and set: dst: <leaf VTEP>
  pw-an1:
    description: Access pseudowire headend
    enabled: true
    mtu: 1508
    pseudowire:
      transport: vxlan-an1
  loop100:
    description: Subscriber Gateway Loopback
    enabled: true
    address:
      ipv4: [10.255.0.1/32]
```

!!! note "Headend MTU: 1508 for QinQ PPPoE"
    The headend must carry the full inner frame. For QinQ PPPoE at the
    standard 1492-byte MRU, that is 1500 payload + 8 bytes of QinQ
    tags = **1508**; sub-interfaces derive their MTU from it
    automatically. Pair it with a jumbo underlay (`mtu: 9000` end to
    end) - VXLAN adds roughly 50 bytes on top.

## IPoE termination

Identical to a physical access port - only `parent-interface` changes:

```yaml
subscriber-groups:
  groups:
    default:
      vlan-tpid: dot1q
      ipv4-profile: default
      vlans:
        - svlan: "100-110"
          cvlan: any
          interface: loop100
          parent-interface: pw-an1
          access-types: [ipoe]
      aaa-policy: default-policy

ipv4-profiles:
  default:
    gateway: 10.255.0.1
    pools:
      - name: subscriber-pool
        network: 10.255.0.0/16
        priority: 1

dhcp:
  provider: local
```

DHCP DISCOVER decapsulates, classifies onto a `pw-an1` sub-interface,
and terminates locally; OFFER/ACK and all downstream traffic return
through the headend into the tunnel.

## PPPoE termination

```yaml
subscriber-groups:
  groups:
    pppoe:
      vlan-tpid: dot1q
      ipv4-profile: default
      vlans:
        - svlan: "200-210"
          cvlan: any
          interface: loop100
          parent-interface: pw-an1
          access-types: [pppoe]
      aaa-policy: pppoe-policy

aaa:
  auth_provider: local
  policy:
    - name: pppoe-policy
      type: ppp
      format: $agent-remote-id$
      authenticate: true
```

PADI through LCP, authentication, and IPCP all ride the pseudowire;
subscribers negotiate the full 1492-byte MRU thanks to the 1508
headend.

## L2TP LAC handoff

Wholesale PPPoE: subscribers arrive over the VXLAN access, osvbng
proxies LCP/auth and tunnels the session to the ISP's LNS over L2TPv2
on the core side:

```yaml
subscriber-groups:
  groups:
    pppoe-lac:
      vlan-tpid: dot1q
      vlans:
        - svlan: "200-210"
          cvlan: any
          interface: loop100
          parent-interface: pw-an1
          access-types: [lac]
      aaa-policy: pppoe-policy
      l2tp:
        profile: L2TP_LAC_DEFAULT

l2tp:
  tunnel-pools:
    LNS_POOL:
      local-name: bng1
      lns:
        - name: isp-lns
          ipv4: 10.0.0.2
          source-ipv4: 10.0.0.1
          secret: shared
          preference: 100
  profiles:
    L2TP_LAC_DEFAULT:
      session-limit: 1000
      hello-interval: 60s
      tunnel-pool: LNS_POOL

aaa:
  auth_provider: local
  policy:
    # LAC mode: osvbng does not authenticate; the LNS validates CHAP
    # via the proxy-auth AVPs forwarded in ICCN. Local auth is a
    # lookup table mapping the line to Tunnel-* attributes.
    - name: pppoe-policy
      type: ppp
      format: $agent-remote-id$
      authenticate: false
```

PPP frames are bridged in the dataplane between the pseudowire and the
L2TP session in both directions; the LNS terminates PPP and assigns
addressing.

## High availability

All three services combine with SRG-based HA over the same transport:
sessions sync to the standby and restore onto its own headend
sub-interfaces by name, and with EVPN signaling the standby's tunnel is
already programmed from its own discovery, so promotion is
dataplane-ready. See [EVPN-VXLAN Fabric](evpn-fabric.md) for the
anycast-VTEP failover pattern.
