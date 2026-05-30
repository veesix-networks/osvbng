# CGNAT <span class="version-badge">v0.6.0</span>

!!! warning
    CGNAT is under active development and not yet feature-complete. PBA mode is functional for IPoE and PPPoE. Deterministic mode, ALG, IPFIX logging, HA sync, and standalone CGNAT mode are not yet available. This documentation may be incomplete. Full CGNAT support is planned for v0.6.0.

Carrier-Grade NAT enables multiple subscribers to share a smaller pool of public IPv4 addresses. osvbng supports two NAT modes: Port Block Allocation (PBA) and Deterministic. Pools can be assigned per subscriber group or per subscriber via AAA, with NAT bypass for public IP customers and configurable session timeouts.

CGNAT is assigned to subscribers through [subscriber groups](subscriber-groups.md) or [service groups](service-groups.md), and can be overridden per subscriber via AAA.

## Outside interfaces

Each pool declares its own `outside_interfaces`: the L3 interface(s) facing the upstream network. The listed interfaces resolve to a single outside VRF (FIB table) per pool, which the CGNAT plugin uses to install return-direction FIB entries for that pool's outside prefixes. Within a single pool, all listed interfaces must share the same VRF.

Different pools may use different outside VRFs, which is what enables wholesale CGNAT.

### Single uplink

```yaml
cgnat:
  pools:
    residential:
      outside_interfaces:
        - eth2
      mode: pba
      inside-prefixes:
        - prefix: 100.64.0.0/16
      outside-addresses:
        - 203.0.113.0/28
```

### Multiple uplinks for one pool (ECMP / VPC topology)

For deployments with two L3 subinterfaces on a LAG facing a VPC / MC-LAG peer pair (each running its own OSPF adjacency to a different upstream SVI), list both subinterfaces under the pool. Both must be in the same VRF so OSPF can ECMP across them and the upstream switches can hash return traffic onto either link.

```yaml
cgnat:
  pools:
    residential:
      outside_interfaces:
        - bond0.100
        - bond0.101
      mode: pba
      inside-prefixes:
        - prefix: 100.64.0.0/16
      outside-addresses:
        - 203.0.113.0/28
```

Return traffic is classified by destination match against the pool's outside prefixes installed in the outside VRF, not by ingress interface. Asymmetric replies (forward out one subinterface, reply back on the other) are handled automatically by the 5-tuple session lookup.

### Wholesale CGNAT (multiple ISP customers)

Hosting NAT services for multiple downstream ISP customers — each with their own inside VRF, public address allocation, and upstream peering — is expressed as one pool per customer. Inside-VRF isolation lets overlapping subscriber address space (e.g. each ISP using 100.64.0.0/16) coexist; outside-VRF isolation lets each ISP's pool addresses be advertised only to that ISP's transit.

```yaml
cgnat:
  pools:
    ispA:
      outside_interfaces:
        - bond0.100
        - bond0.101
      mode: pba
      inside-prefixes:
        - prefix: 100.64.0.0/16
          vrf: ispA-inside
      outside-addresses:
        - 203.0.113.0/24
    ispB:
      outside_interfaces:
        - bond0.200
        - bond0.201
      mode: pba
      inside-prefixes:
        - prefix: 100.64.0.0/16
          vrf: ispB-inside
      outside-addresses:
        - 198.51.100.0/24
```

The CGNAT plugin keys mappings on `(inside_ip, inside_fib_index)` and stores per-pool outside FIB indices, so sessions from ISP A and ISP B are isolated end-to-end even with overlapping inside addresses. Subscribers map to a specific pool via [subscriber groups](subscriber-groups.md) or [service groups](service-groups.md).

An outside interface may appear in more than one pool's list, but only if those pools target the same outside VRF.

### Validation

osvbng rejects the configuration at startup if:

- Any configured pool is missing `outside_interfaces`.
- A pool's listed interfaces resolve to different VRFs.
- An outside interface in any pool is also a subscriber access interface (subscriber and outside roles must not overlap on the same physical interface).
- A pool's `outside-addresses` overlap with an IP address owned by a local interface (the BNG's own control-plane traffic would otherwise be intercepted by the NAT path).
- A legacy top-level `outside_interface` or `outside_interfaces` field is set (the field moved into each pool block).

## Pools

Pools define the translation parameters: which subscriber address ranges to translate, which public addresses to use, and how ports are allocated.

### Pool fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `mode` | string | `pba` | `pba` or `deterministic` |
| `inside-prefixes` | list | required | Subscriber address ranges to translate |
| `outside-addresses` | list | required | Public NAT addresses (IPs or CIDR prefixes) |
| `block-size` | uint16 | `512` | Ports per block (PBA mode) |
| `max-blocks-per-subscriber` | uint8 | `4` | Maximum port blocks per subscriber (PBA mode) |
| `max-sessions-per-subscriber` | uint32 | `2000` | Maximum concurrent sessions per subscriber |
| `address-pooling` | string | `paired` | `paired` keeps all sessions on the same outside IP; `arbitrary` allows multiple |
| `filtering` | string | `endpoint-independent` | `endpoint-independent` or `endpoint-dependent` |
| `port-range` | string | `1024-65535` | Usable port range |
| `port-reuse-timeout` | uint16 | `120` | Seconds before a freed port can be reused |
| `excluded-addresses` | list | - | Outside addresses to exclude from allocation |
| `ports-per-subscriber` | uint16 | - | Fixed port count per subscriber (deterministic mode) |
| `network-route-policy` | string | - | [Route-policy](routing-policies.md) applied when advertising outside addresses into BGP |
| `timeouts` | object | see below | Per-protocol session timeouts |

### Inside prefixes

Each entry specifies a subscriber address range. Optionally, a VRF can be specified for multi-VRF deployments:

```yaml
inside-prefixes:
  - prefix: 100.64.0.0/16
  - prefix: 10.0.0.0/8
    vrf: CUSTOMER-A
```

### Timeouts

| Field | Default | Description |
|-------|---------|-------------|
| `tcp-established` | `7200` | Established TCP connections (seconds) |
| `tcp-transitory` | `240` | TCP SYN/FIN/RST transitory states (seconds) |
| `udp` | `300` | UDP sessions (seconds) |
| `icmp` | `60` | ICMP sessions (seconds) |

## Modes

### PBA (Port Block Allocation)

Each subscriber receives a block of ports on a shared public IP address. If the initial block is exhausted, additional blocks are allocated up to `max-blocks-per-subscriber`.

```yaml
cgnat:
  pools:
    residential:
      mode: pba
      inside-prefixes:
        - prefix: 100.64.0.0/16
      outside-addresses:
        - 203.0.113.0/28
      block-size: 512
      max-blocks-per-subscriber: 2
      max-sessions-per-subscriber: 2000
      address-pooling: paired
      filtering: endpoint-independent
      timeouts:
        tcp-established: 7200
        tcp-transitory: 240
        udp: 300
        icmp: 60
```

### Deterministic

Each subscriber's public IP and port range is algorithmically computed from the inside address, requiring no per-subscriber state. This simplifies logging compliance (RFC 7422) since mappings can be derived from the subscriber IP and a timestamp.

```yaml
cgnat:
  pools:
    residential:
      mode: deterministic
      inside-prefixes:
        - prefix: 100.64.0.0/16
      outside-addresses:
        - 203.0.113.0/28
      ports-per-subscriber: 512
```

!!! note
    Deterministic mode translation is not yet available. Configuration is accepted but translation will not occur until a future release.

## Subscriber Assignment

### Via subscriber group

Assign a CGNAT pool to all subscribers in a group:

```yaml
subscriber-groups:
  groups:
    default:
      access-types: [ipoe]
      ipv4-profile: shared-pool
      vlans:
        - svlan: "100-110"
          cvlan: any
          interface: loop100
      aaa-policy: default-policy
      cgnat:
        policy: residential
```

### Via service group

Service groups support both policy assignment and bypass. See [service groups](service-groups.md) for full details on attribute merging and AAA override.

```yaml
service-groups:
  residential:
    cgnat:
      policy: residential

  business:
    cgnat:
      bypass: true
```

### Bypass

When `bypass: true` is set on a service group, the subscriber's address is added to the NAT bypass table. Traffic from bypass subscribers passes through without translation. This is used for business customers with public IP addresses.

### AAA override

AAA can assign a CGNAT policy or enable bypass per subscriber, overriding the subscriber group or service group defaults.

## Outside address advertisement

By default, osvbng automatically announces outside pool prefixes via BGP. A blackhole route is installed for each prefix and advertised as a BGP network statement through the configured [BGP session](protocols.md).

For more flexible routing policies (e.g. selective advertisement, communities, route-maps), you may prefer to disable automatic advertisement and configure the outside prefix routes manually in the [protocols](protocols.md) section. This gives full control over how the outside addresses are announced to upstream peers.

## Show commands

| Path | Description |
|------|-------------|
| `cgnat.pools` | Pool configuration and allocation statistics |
| `cgnat.sessions` | Active NAT translations (5-tuple flows), filterable by inside/outside/remote IP + port + protocol |
| `cgnat.mappings` | Subscriber-to-pool port-block mappings |
| `cgnat.statistics` | Per-pool counters |
| `cgnat.lookup` | Reverse lookup: find a subscriber by outside IP and port |

The `cgnat.sessions` dump is filtered and windowed by the dataplane. Page with
`cursor`/`limit` and follow `next_cursor` until `has_more` is false; `total` is
the global live session count. Example:
`?inside-ip=100.64.0.2&proto=tcp&limit=100`.

## Operational commands

| Path | Description |
|------|-------------|
| `cgnat.test-mapping` | Test CGNAT mapping for a given inside IP |

All commands are available via the [northbound API](plugins/northbound-api.md):

```bash
curl http://localhost:8080/api/show/cgnat/pools
curl "http://localhost:8080/api/show/cgnat/sessions?inside-ip=100.64.0.2"
curl http://localhost:8080/api/show/cgnat/mappings
curl http://localhost:8080/api/show/cgnat/statistics
curl "http://localhost:8080/api/show/cgnat/lookup?ip=203.0.113.1&port=2048"
curl -X POST http://localhost:8080/api/oper/cgnat/test-mapping -d '{"inside_ip": "100.64.0.2"}'
```

## Full example

```yaml
cgnat:
  pools:
    residential:
      outside_interfaces:
        - eth2
      mode: pba
      inside-prefixes:
        - prefix: 100.64.0.0/16
      outside-addresses:
        - 203.0.113.0/28
      block-size: 512
      max-blocks-per-subscriber: 2
      max-sessions-per-subscriber: 2000
      address-pooling: paired
      filtering: endpoint-independent
      timeouts:
        tcp-established: 7200
        tcp-transitory: 240
        udp: 300
        icmp: 60

subscriber-groups:
  groups:
    default:
      access-types: [ipoe]
      ipv4-profile: shared-pool
      vlans:
        - svlan: "100-110"
          cvlan: any
          interface: loop100
      aaa-policy: default-policy
      cgnat:
        policy: residential

service-groups:
  business:
    cgnat:
      bypass: true

ipv4-profiles:
  shared-pool:
    gateway: 100.64.0.1
    dns:
      - 8.8.8.8
      - 8.8.4.4
    pools:
      - name: cgnat-pool
        network: 100.64.0.0/16
        priority: 1
    dhcp:
      lease-time: 3600
```

## Restart reconciliation

On `osvbngd` start the CGNAT component reconciles VPP's running pool / inside-prefix / outside-address state against the declared YAML. Each pool is processed as a per-pool transaction (parent then children) and a post-apply re-dump verifies convergence before the daemon proceeds.

| Class | What | Default policy |
|-------|------|----------------|
| Missing in VPP | Pool / prefix / address in YAML, not in VPP | Add |
| Identical | Pool / prefix / address in both, same params | No-op |
| Soft drift | Pool params differ on `timeouts`, `max-sessions-per-subscriber`, ALG bitmask, `port-reuse-timeout` | Plugin updates in place, mappings preserved |
| Hard drift | Pool params differ on `mode`, `block-size`, `port-range`, `address-pooling`, `filtering`; or outside-VRF flip | Replace (del + add) — drops active mappings |
| Orphan in VPP | Pool / prefix / address VPP has but no YAML entry | Remove |

### Reconcile block

```yaml
cgnat:
  reconcile:
    on_divergence: reconcile      # reconcile (default) | fail
    drop_orphans: true            # bool (default true)
    allow_pool_disruption: false  # bool (default false)
```

- **`on_divergence: fail`** — apply reconcile actions, log every divergence, then return error from `Start()`. Useful in high-assurance environments where any drift is treated as a bug. Note: this is converge-and-notify, not preflight-fail-no-change.
- **`drop_orphans: false`** — VPP-only entries are kept (with WARN). Useful during brownfield migration when another tool may own some entries.
- **`allow_pool_disruption: false`** *(default)* — any reconcile action that would drop active subscriber NAT state aborts `Start()` with an actionable error naming the affected pools and mapping counts. Operator must explicitly set `true` (or schedule a maintenance window) to apply destructive changes. This is the safety gate that prevents an innocent-looking `block-size` edit from dropping a production-load of subscribers on restart.

### Operator workflow for a destructive change

```
1. Edit cgnat.pools.<name>.block-size in osvbng.yaml.
2. systemctl restart osvbng
   → Daemon refuses to start, journalctl shows:
     "cgnat: reconcile: refusing to disrupt active subscriber NAT state.
      Set cgnat.reconcile.allow_pool_disruption: true to proceed..."
3. Schedule maintenance window. Set cgnat.reconcile.allow_pool_disruption: true.
4. systemctl restart osvbng
   → Daemon WARN-logs "replace pool" with drift_fields and dropped_mappings count.
   → New mappings allocate under the new block size.
5. Optionally remove allow_pool_disruption from the config after.
```


