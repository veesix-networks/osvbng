# Dataplane

Dataplane and DPDK configuration.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `dpdk` | [DPDK](#dpdk-configuration) | DPDK-specific configuration | |
| `statseg` | [StatsSegment](#stats-segment) | Stats segment configuration | |

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
