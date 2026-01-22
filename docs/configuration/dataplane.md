# Dataplane

Dataplane socket paths and DPDK configuration.

| Field | Type | Description |
|-------|------|-------------|
| `dp_api_socket` | string | Dataplane API socket path |
| `punt_socket_path` | string | Punt socket for control packets |
| `arp_punt_socket_path` | string | ARP punt socket |
| `memif_socket_path` | string | Memif socket path |
| `dpdk` | object | DPDK-specific configuration |

## DPDK Configuration

| Field | Type | Description |
|-------|------|-------------|
| `uio_driver` | string | UIO driver: `vfio-pci`, `uio_pci_generic` |
| `devices` | array | List of DPDK devices |
| `socket_mem` | string | Hugepage memory per socket (e.g., `1024,1024`) |
| `no_multi_seg` | bool | Disable multi-segment mbufs |
| `no_tx_checksum_offload` | bool | Disable TX checksum offload |
| `enable_tcp_udp_checksum` | bool | Enable TCP/UDP checksum |
| `max_simd_bitwidth` | int | Max SIMD bitwidth (e.g., `512`) |

## DPDK Devices

| Field | Type | Description |
|-------|------|-------------|
| `pci` | string | PCI address (e.g., `0000:05:00.0`) |
| `name` | string | Interface name (e.g., `eth1`) |
| `options.num_rx_queues` | int | Number of RX queues |
| `options.num_tx_queues` | int | Number of TX queues |
| `options.num_rx_desc` | int | RX descriptor ring size |
| `options.num_tx_desc` | int | TX descriptor ring size |
| `options.tso` | bool | Enable TCP segmentation offload |
| `options.devargs` | string | Additional device arguments |

## Example

```yaml
dataplane:
  dpdk:
    devices:
      - pci: "0000:05:00.0"
        name: eth1
      - pci: "0000:06:00.0"
        name: eth2
    socket_mem: "1024,1024"
```
