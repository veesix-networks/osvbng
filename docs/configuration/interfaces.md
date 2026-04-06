# Interfaces

Network interface configuration. Each key in the `interfaces` map is an interface name.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `name` | string | Interface name | `eth1` |
| `description` | string | Human-readable description | `Access Interface` |
| `enabled` | bool | Enable the interface | `true` |
| `mtu` | int | MTU size | `9000` |
| `lcp` | bool | Create Linux Control Plane interface | `true` |
| `bng_mode` | string | BNG role: `access` or `core` | `access` |
| `unnumbered` | string | Borrow address from named interface | `loop100` |
| `bond` | [Bond](#bond) | Bond interface configuration (DPDK only) | |
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

## Example

```yaml
interfaces:
  eth1:
    name: eth1
    description: Access Interface
    enabled: true
    bng_mode: access

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
