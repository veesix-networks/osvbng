# Dataplane

Dataplane and DPDK configuration.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `rx_mode` | string | Interface RX mode: `polling`, `interrupt`, or `adaptive` | `polling` |
| `dpdk` | [DPDK](#dpdk-configuration) | DPDK-specific configuration | |
| `statseg` | [StatsSegment](#stats-segment) | Stats segment configuration | |

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
