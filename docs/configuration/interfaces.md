# Interfaces

Network interface configuration. Each key in the `interfaces` map is an interface name.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `name` | string | Interface name | `eth1` |
| `description` | string | Human-readable description | `Access Interface` |
| `enabled` | bool | Enable the interface | `true` |
| `mtu` | int | MTU size | `9000` |
| `lcp` | bool | Create Linux Control Plane interface | `true` |
| `unnumbered` | string | Borrow address from named interface | `loop100` |
| `bond` | [Bond](#bond) | Bond interface configuration (DPDK only) | |
| `vxlan` | [VXLAN](#vxlan) | VXLAN tunnel configuration | |
| `pseudowire` | [Pseudowire](#pseudowire) | Pseudowire headend configuration | |
| `address` | [Address](#address) | IP address configuration | |
| `subinterfaces` | [Subinterface](#sub-interfaces) | Sub-interface configuration | |
| `ipv6` | [IPv6](#ipv6) | IPv6 configuration | |
| `arp` | [ARP](#arp) | ARP configuration | |

## Address

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `ipv4` | array | IPv4 addresses (CIDR notation) | `[10.255.0.1/32]` |
| `ipv6` | array | IPv6 addresses (CIDR notation) | `[2001:db8::1/128]` |

## Sub-interfaces

Sub-interfaces are configured as a list under the parent interface. Each entry requires an `id` (the VPP sub-interface ID) and a `vlan` (the outer VLAN to match).

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `id` | int | Sub-interface ID | `100` |
| `vlan` | int | Outer VLAN ID (1-4094) | `100` |
| `inner-vlan` | int | Inner VLAN ID for double-tag match (1-4094) | `200` |
| `vlan-tpid` | string | Outer VLAN TPID: `dot1q` or `dot1ad`. Defaults to `dot1ad` for double-tagged sub-interfaces (IEEE 802.1ad), `dot1q` for single-tagged | `dot1ad` |
| `enabled` | bool | Enable the sub-interface | `true` |
| `description` | string | Human-readable description | `Customer A` |
| `mtu` | int | MTU override (auto-derived from parent if not set) | `1504` |
| `lcp` | bool | Create Linux Control Plane interface (only needed for addressless interfaces, e.g. unnumbered core interfaces for FRR routing) | `true` |
| `vrf` | string | Bind to VRF | `CUSTOMER-A` |
| `address` | [Address](#address) | IP address configuration | |
| `ipv6` | [IPv6](#ipv6) | IPv6 configuration | |
| `arp` | [ARP](#arp) | ARP configuration | |
| `unnumbered` | string | Borrow address from named interface | `loop100` |
| `bng` | [BNG](#sub-interface-bng) | BNG configuration | |

!!! info "Automatic sub-interface management"
    When using the BNG functionality of osvbng with subscriber groups, sub-interfaces are automatically deployed and managed based on the VLAN matching rules. You do not need to manually configure sub-interfaces in this section.

!!! note "Automatic MTU"
    If `mtu` is not set, the sub-interface MTU is automatically derived from the parent interface: parent MTU plus 4 bytes for single-tag (802.1q), or plus 8 bytes for double-tag (QinQ). Set `mtu` explicitly to override.

!!! note "Automatic LCP"
    When an IPv4 or IPv6 address is configured on a sub-interface, an LCP (Linux Control Plane) pair is automatically created. You only need to set `lcp: true` explicitly for addressless sub-interfaces that need Linux visibility (e.g., unnumbered core interfaces for FRR routing protocols).

!!! warning "VLAN matching flags are immutable"
    VPP does not support modifying sub-interface VLAN matching flags after creation. Changing `vlan`, `inner-vlan`, or `vlan-tpid` on an existing sub-interface requires a restart to take effect.

### VLAN Matching Modes

| Config | Matching |
|--------|----------|
| `vlan: 100` | Single tag, outer dot1q |
| `vlan: 100, inner-vlan: 200` | Double tag exact match, outer dot1ad |
| `vlan: 100, inner-vlan: 200, vlan-tpid: dot1q` | Double tag exact match, outer dot1q |
| BNG subscriber sub-interface | Outer S-VLAN match, any inner C-VLAN, outer dot1ad |

### Sub-interface BNG

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `mode` | string | BNG mode: `ipoe`, `ipoe-l3`, `pppoe`, `lac`, `lns` | `pppoe` |

## IPv6

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `enabled` | bool | Enable IPv6 | `true` |
| `multicast` | bool | Enable IPv6 multicast | `true` |
| `ra` | [RA](#router-advertisement) | Router Advertisement configuration | |

### Router Advertisement

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `managed` | bool | Set Managed (M) flag in RA | `true` |
| `other` | bool | Set Other (O) flag in RA | `true` |
| `router-lifetime` | int | Router lifetime in seconds | `1800` |
| `max-interval` | int | Max RA interval in seconds | `600` |
| `min-interval` | int | Min RA interval in seconds | `200` |

## ARP

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `enabled` | bool | Enable ARP | `true` |

## Bond

Bond interface configuration for link aggregation. In DPDK deployments, bonds are created inside VPP. In AF_PACKET (Docker) deployments, bonds are managed by the host OS — configure the bond interface by name without a `bond` section.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `mode` | string | Bond mode | `lacp` |
| `members` | array | Member interfaces (string or object) | |
| `load-balance` | string | Load balancing algorithm (XOR/LACP only) | `l23` |
| `gso` | bool | Enable Generic Segmentation Offload | `true` |
| `mac-address` | string | Custom MAC address for the bond | `02:00:00:00:00:01` |

**Bond modes:** `lacp` (default), `round-robin`, `active-backup`, `xor`, `broadcast`

**Load balance algorithms:** `l2` (default), `l23`, `l34` — only valid for `lacp` and `xor` modes.

### Bond Members

Members can be specified as a simple string or as an object with per-member LACP settings:

```yaml
members:
  - TenGigabitEthernet0/0/0                  # string shorthand (active, short timeout)
  - name: TenGigabitEthernet0/0/1            # object with LACP settings
    passive: false
    long-timeout: false
```

| Field | Type | Description | Default |
|-------|------|-------------|---------|
| `name` | string | Member interface name | |
| `passive` | bool | LACP passive mode (don't initiate) | `false` |
| `long-timeout` | bool | 90 second timeout (vs 3 second default) | `false` |

!!! info "AF_PACKET (Docker) deployments"
    When running osvbng in Docker with AF_PACKET, bond interfaces are managed by the host operating system (e.g., Linux bonding). Simply reference the bond interface by name (e.g., `bond0`) in your configuration without a `bond` section — VPP will attach to it as a regular host interface.

## VXLAN

A `vxlan` section turns the interface into a point-to-point VXLAN tunnel. The tunnel is a normal named interface afterward: it can be referenced as an l2gw `handoff-groups.<name>.interface` and as a subscriber-group `parent-interface` for `l2gw` access types, exactly like a physical port.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `src` | string | Local VTEP address (must be an address VPP owns) | `10.254.0.1` |
| `src-interface` | string | Take the VTEP address from this interface's first IPv4 address (alternative to `src`) | `loop0` |
| `dst` | string | Remote VTEP address (static tunnels; mutually exclusive with `signaling: evpn`) | `10.254.0.101` |
| `vni` | int | VXLAN Network Identifier (1-16777215) | `10101` |
| `signaling` | string | Set to `evpn` to learn the remote VTEP via BGP EVPN instead of configuring `dst` | `evpn` |

At least one of `src` or `src-interface` must be set (`src-interface` resolves to `src` at load time). IPv4 and IPv6 underlays are supported; `src` and `dst` must be the same address family.

With `signaling: evpn` the tunnel's VNI is advertised as an EVPN type-3 (IMET) route and the remote VTEP is discovered from the fabric rather than provisioned. This requires the BGP [`l2vpn-evpn` address family](protocols.md#bgp-l2vpn-evpn) to be enabled with `advertise-all-vni`. Each EVPN-signaled tunnel must use a unique VNI.

Decapsulated frames re-enter the RX feature pipeline as if they arrived on a physical port (via the `osvbng_tunnel` VPP plugin), so l2gw circuit switching and the DHCP trigger snoop work on tunnels unchanged. Encapsulation uses a flow-hash UDP source port, giving the underlay per-flow entropy for ECMP, LAG hashing, and receiver-side RSS.

!!! note "Underlay MTU"
    VXLAN adds roughly 50 bytes of encapsulation on top of the inner frame (which itself can carry QinQ tags). Run a jumbo underlay: set `mtu: 9000` on the underlay interfaces end to end.

!!! warning "Current limitations"
    VXLAN tunnel interfaces currently carry L2 wholesale (l2gw) service only. VLAN sub-interfaces and direct IPoE/PPPoE subscriber termination on tunnels are not yet supported, and the underlay must live in the default VRF.

```yaml
interfaces:
  loop0:
    enabled: true
    address:
      ipv4: [10.254.0.1/32]     # VTEP source
  vxlan-an1:
    description: Access operator NNI
    enabled: true
    vxlan:
      src-interface: loop0
      dst: 10.254.0.101         # leaf / remote VTEP
      vni: 10101
```

## Pseudowire

A `pseudowire` section creates a pseudowire headend (pw-ether style): a virtual access port backed by a tunnel transport. Decapsulated frames are re-attributed to the headend, so VLAN sub-interfaces, subscriber-groups, and full IPoE/PPPoE/LAC termination work on it exactly as on a physical port, while all traffic rides the transport tunnel.

!!! note "Terminology"
    Most vendors use *pseudowire* to mean specifically an MPLS-signaled circuit (LDP-signaled L2VPN or EVPN-VPWS) terminating on a headend interface such as Cisco `PW-Ether`. On osvbng the term is deliberately broader: a pseudowire is any point-to-point L2 service delivered over a tunnel transport and presented as a virtual access interface. Today that transport is a VXLAN tunnel; MPLS-based transports (EVPN-VPWS, SR-MPLS) and SRv6 are planned to slot into the same `transport` field. The headend semantics - VLAN sub-interfaces, subscriber termination, S/C-VLAN matching - are identical regardless of the transport underneath, which keeps the configuration model stable as transports are added.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `transport` | string | Tunnel interface carrying this headend | `vxlan-an1` |
| `mac-address` | string | Pin the headend MAC (set identically on both HA nodes so subscribers keep their resolved gateway MAC across switchover) | `02:00:00:00:a1:01` |

One headend per transport tunnel. A tunnel referenced as a pseudowire transport cannot also be used as an l2gw NNI or subscriber-group parent directly.

```yaml
interfaces:
  vxlan-an1:
    enabled: true
    vxlan:
      src-interface: loop0
      dst: 10.254.0.101
      vni: 10101
  pw-an1:
    description: Access operator 1 headend
    enabled: true
    pseudowire:
      transport: vxlan-an1

subscriber-groups:
  groups:
    residential-an1:
      vlans:
        - svlan: "100-4094"
          cvlan: any
          interface: loop100
          parent-interface: pw-an1
          access-types: [ipoe]
```

The subscriber-facing MAC is the headend's own MAC; S/C-VLAN matching, NAS-Port-Id, unnumbered gateways, and HA session restore behave identically to physical parents.

For HA pairs, give both nodes identical tunnel and headend configuration, pin `pseudowire.mac-address` to the SRG `virtual_mac` on both nodes (so the subscriber gateway MAC never changes across switchover), present one anycast VTEP address toward the access network, and list the underlay interface in `ha.srgs.<srg>.interfaces` - the SRG virtual MAC then gates which node accepts and decapsulates tunnel traffic. Sessions restore onto the peer's own headend sub-interfaces on promotion by name. The transport underlay must be jumbo (VXLAN ~50B overhead on QinQ frames). Size the headend MTU exactly like a physical access port: QinQ PPPoE at full 1492 MRU needs `mtu: 1508` on the parent (IP + 8 PPPoE/PPP + 8 QinQ), otherwise full-size subscriber packets get inner-fragmented.

## Example

```yaml
interfaces:
  eth1:
    name: eth1
    description: Access Interface
    enabled: true
  eth2:
    name: eth2
    description: Core Interface
    enabled: true
    lcp: true
    subinterfaces:
      - id: 100
        vlan: 100
        enabled: true
        lcp: true
        vrf: CUSTOMER-A
        address:
          ipv4:
            - 10.0.100.1/24
        description: "Customer A VRF-lite"

  loop100:
    name: loop100
    description: Subscriber Gateway
    enabled: true
    lcp: true
    address:
      ipv4:
        - 10.255.0.1/32
    ipv6:
      enabled: true
      ra:
        managed: true
        other: true
        router-lifetime: 1800
```

### DPDK Bond Example

```yaml
interfaces:
  TenGigabitEthernet0/0/0:
    description: Core link 1
    enabled: true
  TenGigabitEthernet0/0/1:
    description: Core link 2
    enabled: true
  bond0:
    description: Core LACP bond
    enabled: true
    lcp: true
    bond:
      mode: lacp
      load-balance: l23
      gso: true
      members:
        - TenGigabitEthernet0/0/0
        - TenGigabitEthernet0/0/1
```

### AF_PACKET Bond Example

In Docker deployments, bonding is managed by the host OS. The `setup-interfaces.sh` script bridges the container's veth pair to the host bond interface. osvbng sees the container interface (e.g., `eth1`), and the `name` field can be used to reference it as `bond0` inside osvbng.

```bash
# Host side: bridge container eth1 to host bond0
./setup-interfaces.sh osvbng eth0:br-mgmt eth1:bond0
```

```yaml
# osvbng config — eth1 is bridged to host bond0, renamed to bond0 inside VPP
interfaces:
  eth1:
    name: bond0
    description: Core bond (managed by host OS)
    enabled: true
    lcp: true
```
