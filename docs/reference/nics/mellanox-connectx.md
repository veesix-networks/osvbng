# Mellanox / NVIDIA ConnectX

ConnectX-4, ConnectX-5, ConnectX-6, ConnectX-7. The recommended NIC
family for osvbng production deployments.

## Hardware

| Family | Speeds | Driver |
|---|---|---|
| ConnectX-4 | up to 100 GbE | mlx5_core / mlx5 PMD |
| ConnectX-5 | up to 100 GbE | mlx5_core / mlx5 PMD |
| ConnectX-6 | up to 200 GbE | mlx5_core / mlx5 PMD |
| ConnectX-7 | up to 400 GbE | mlx5_core / mlx5 PMD |

PCI vendor ID: `15b3`.

## Driver strategy

Bifurcated. Kernel `mlx5_core` stays bound; the DPDK `mlx5` PMD
accesses the hardware via the Verbs API. Data plane is fully DPDK
(packets bypass the kernel) but link state, ethtool, and firmware
remain accessible to Linux.

Implications:

- Interfaces appear in both Linux and the dataplane.
- Linux interfaces should remain unconfigured (no IP).
- Works in VMs with PCI passthrough; no guest IOMMU required.
- Full line-rate DPDK performance.

Requirements:

- `rdma-core` / `libibverbs` packages
- `mlx5_core` kernel module


