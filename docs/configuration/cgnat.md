# CGNAT <span class="version-badge">v0.6.0</span>

!!! warning
    CGNAT is under active development and not yet feature-complete. PBA mode is functional for IPoE and PPPoE. Deterministic mode, ALG, IPFIX logging, HA sync, and standalone CGNAT mode are not yet available. This documentation may be incomplete. Full CGNAT support is planned for v0.6.0.

Carrier-Grade NAT enables multiple subscribers to share a smaller pool of public IPv4 addresses. osvbng supports two NAT modes: Port Block Allocation (PBA) and Deterministic. Pools can be assigned per subscriber group or per subscriber via AAA, with NAT bypass for public IP customers and configurable session timeouts.

CGNAT is assigned to subscribers through [subscriber groups](subscriber-groups.md) or [service groups](service-groups.md), and can be overridden per subscriber via AAA.

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
      access-type: ipoe
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
| `cgnat.sessions` | Active subscriber mappings |
| `cgnat.statistics` | Per-pool counters |
| `cgnat.lookup` | Reverse lookup: find a subscriber by outside IP and port |

## Operational commands

| Path | Description |
|------|-------------|
| `cgnat.test-mapping` | Test CGNAT mapping for a given inside IP |

All commands are available via the [northbound API](plugins/northbound-api.md):

```bash
curl http://localhost:8080/api/show/cgnat/pools
curl http://localhost:8080/api/show/cgnat/sessions
curl http://localhost:8080/api/show/cgnat/statistics
curl "http://localhost:8080/api/show/cgnat/lookup?ip=203.0.113.1&port=2048"
curl -X POST http://localhost:8080/api/oper/cgnat/test-mapping -d '{"inside_ip": "100.64.0.2"}'
```

## Full example

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

subscriber-groups:
  groups:
    default:
      access-type: ipoe
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
