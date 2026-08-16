# Dataplane

Dataplane and DPDK configuration.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `rx_mode` | string | Interface RX mode: `polling`, `interrupt`, or `adaptive` | `polling` |
| `main-core` | int | Core for VPP's main thread. Auto-resolved when unset | `0` |
| `workers` | string | Core set for VPP workers. Auto-resolved when unset | `1-5` |
| `cp-cores` | string | Core set for the Go control plane. Auto-resolved when unset | `6-7` |
| `poll-sleep-usec` | int | Microseconds an idle worker sleeps between dispatch loops. Default `100` (low idle CPU); `0` polls continuously for lowest latency | `0` |
| `dpdk` | [DPDK](#dpdk-configuration) | DPDK-specific configuration | |
| `statseg` | [StatsSegment](#stats-segment) | Stats segment configuration | |

## CPU Cores

Each host resolves core placement locally at startup from the cores actually available to it (container cpusets are respected), so the same config works across hosts with different core counts. Unset fields follow the auto layout:

| Cores | VPP main | VPP workers | Control plane |
|-------|----------|-------------|---------------|
| 1 | 0 | - | shared (0) |
| 2 | 0 | 1 | shared (0) |
| 3 | 0 | 1 | 2 |
| 4 | 0 | 1-2 | 3 |
| 5-7 | 0 | 1-(N-2) | N-1 |
| 8+ | 0 | 1-(N-3) | (N-2)-(N-1) |

Explicit sets override the auto layout per field and use the same syntax everywhere: ranges and commas, for example `2-3,34-35`. The control plane sizes `GOMAXPROCS` to its resolved set and is pinned to it, so giving it more cores is a config change, not a build change. Resolution is topology-blind: on a multi-socket host, expressing NUMA placement (workers beside their NICs, control-plane cores on a chosen socket) is done with explicit sets.

```yaml
dataplane:
  main-core: 0
  workers: 1-5
  cp-cores: 6-7
```

## RX Mode

Controls how VPP polls AF_PACKET host interfaces for incoming packets.

- **`interrupt`** (default): Workers sleep until the kernel signals a packet arrival. Near-zero CPU when idle.
- **`polling`**: Each worker thread busy-polls its assigned interfaces, pinning its core at 100% CPU even when idle. This is by design in VPP to maximise time spent packet processing and minimise latency.
- **`adaptive`**: VPP dynamically switches between polling and interrupt based on traffic load.

LCP tap interfaces are always set to interrupt mode regardless of this setting. These interfaces exist to pass control plane packets (BGP, OSPF, ARP, ND, etc.) into the kernel for FRR and do not benefit from polling.

This setting does not affect DPDK interfaces, which manage their own RX mode independently.

```yaml
dataplane:
  rx_mode: polling
```

## Worker Poll Sleep

`poll-sleep-usec` caps how long an idle worker sleeps between dispatch loops, in microseconds. It trades idle CPU against latency.

- **`100`** (default): idle workers sleep up to 100µs between polls, keeping CPU low when there is no traffic. Best for dev, test, and shared hosts.
- **`0`**: workers never sleep when idle and poll continuously, giving the lowest latency at the cost of pinning each worker core at 100% CPU.

Set `poll-sleep-usec: 0` on production deployments. Under load workers are always busy, so the sleep only affects idle workers, but with several workers a low-rate flow (for example ICMP) landing on a worker that is momentarily idle can otherwise wait up to the sleep interval before that worker wakes to handle it. `0` removes that tail. DPDK workers are unaffected under load; this only changes idle-worker behaviour.

```yaml
dataplane:
  poll-sleep-usec: 0
```

## Stats Segment

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `size` | string | Stats segment size | `64m` |
| `page-size` | string | Page size | `2m` |
| `per-node-counters` | bool | Enable per-node counters | `true` |

## DPDK Configuration

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `uio_driver` | string | UIO driver: `vfio-pci`, `uio_pci_generic` | `vfio-pci` |
| `devices` | [DPDKDevice](#dpdk-devices) | List of DPDK devices | |
| `dev_defaults` | [DPDKDeviceOptions](#dpdk-device-options) | Default device options applied to all devices | |
| `socket_mem` | string | Hugepage memory per socket | `1024,1024` |
| `no_multi_seg` | bool | Disable multi-segment mbufs | `false` |
| `no_tx_checksum_offload` | bool | Disable TX checksum offload | `false` |
| `enable_tcp_udp_checksum` | bool | Enable TCP/UDP checksum | `false` |
| `max_simd_bitwidth` | int | Max SIMD bitwidth | `512` |

## DPDK Devices

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `pci` | string | PCI address | `0000:05:00.0` |
| `name` | string | Interface name | `eth1` |
| `options` | [DPDKDeviceOptions](#dpdk-device-options) | Per-device options | |

## DPDK Device Options

Applies to both per-device `options` and global `dev_defaults`.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `num_rx_queues` | int | Number of RX queues | `2` |
| `num_tx_queues` | int | Number of TX queues | `2` |
| `num_rx_desc` | int | RX descriptor ring size | `1024` |
| `num_tx_desc` | int | TX descriptor ring size | `1024` |
| `tso` | bool | Enable TCP segmentation offload | `false` |
| `devargs` | string | Additional device arguments | |
| `rss_queues` | string | RSS queue configuration | |
| `no_rx_interrupt` | bool | Disable RX interrupts | `false` |

## Example

```yaml
dataplane:
  statseg:
    size: 64m
    per-node-counters: true
  dpdk:
    dev_defaults:
      num_rx_queues: 2
      num_tx_queues: 2
    devices:
      - pci: "0000:05:00.0"
        name: eth1
      - pci: "0000:06:00.0"
        name: eth2
        options:
          num_rx_queues: 4
    socket_mem: "1024,1024"
```
