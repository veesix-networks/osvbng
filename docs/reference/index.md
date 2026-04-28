# Reference

Hardware reference for osvbng deployments: dataplane modes, supported
NICs, validated server designs, and per-vendor caveats.

## Dataplane modes

| Mode | Performance | Use case | NIC requirement |
|---|---|---|---|
| **DPDK** | Line-rate | Production, high-throughput | Supported NICs (see vendor matrix below) |
| **AF_XDP** | High | Production, simpler setup | Any XDP-capable NIC (coming soon) |
| **AF_PACKET** | Standard | Development, any NIC | Any Linux NIC |

### DPDK

Recommended for production. Vendor-specific driver binding strategy
(see matrix below). Configure devices in `osvbng.yaml`:

```yaml
dataplane:
  dpdk:
    devices:
      - pci: "0000:05:00.0"
        name: eth1
      - pci: "0000:06:00.0"
        name: eth2
```

### AF_XDP (coming soon)

eBPF/XDP for high performance while keeping interfaces visible in
Linux. Right pick where DPDK driver binding is impractical.

### AF_PACKET

Works with any Linux NIC. For development and testing.

## Vendor support matrix

| Vendor | Supported families | Driver strategy | Linux visibility | Notes |
|---|---|---|---|---|
| [Mellanox / NVIDIA](nics/mellanox-connectx.md) | ConnectX-4, 5, 6, 7 | Bifurcated (`mlx5`) | Yes | Recommended for production. No guest IOMMU required in VMs. |
| [Intel X710 family](nics/intel-x710.md) | X710, XXV710, XL710 | `vfio-pci` (`i40e`) | No (after bind) | Requires `intel_iommu=on iommu=pt`. ~37 Mpps small-packet ceiling. |
| [Intel E810](nics/intel-e810.md) | E810-CQDA2, E810-XXV | `vfio-pci` (`ice`) | No (after bind) | Requires IOMMU. DDP profile for protocol-aware features. |
| Other | — | `vfio-pci` (fallback) | No | Adding a vendor: see [Adding vendor support](nics/adding-vendor-support.md). |

## Hardware matrix

Validated server designs with measured throughput and subscriber
counts: see [Hardware Matrix](hardware-matrix.md).

## Per-NIC reference

Hardware specs, throughput limits, firmware/kernel requirements, and
known caveats per NIC family: see [NICs](nics/index.md).

## Bind strategies

### Bifurcated

Kernel driver stays bound; DPDK PMD accesses hardware via the Verbs
API. Used by Mellanox.

- Data plane is fully DPDK
- Control plane (link state, ethtool, firmware) shared with kernel
- Linux interface visible but should be unconfigured (no IP)
- Works in VMs with PCI passthrough; no guest IOMMU required

### vfio-pci

Device unbound from the kernel and bound to `vfio-pci` for exclusive
DPDK ownership. Used by Intel and most other vendors as the default.

- Requires IOMMU enabled (`intel_iommu=on iommu=pt`)
- Device disappears from Linux after bind
- Not suitable for VMs without nested IOMMU

### uio_pci_generic

Fallback when `vfio-pci` is unavailable.

- No IOMMU required
- Less secure (no DMA protection)

## Troubleshooting

**Device not appearing in dataplane.** Check the driver binding:

```bash
ls -la /sys/bus/pci/devices/0000:XX:00.0/driver
```

Mellanox should show `mlx5_core`. Intel and other vfio-bound devices
should show `vfio-pci`.

**`vfio-pci` bind fails with EINVAL (-22).** IOMMU not available.
Either:

1. Enable IOMMU in BIOS and kernel (`intel_iommu=on`)
2. Use noiommu mode (less secure):
   `echo 1 > /sys/module/vfio/parameters/enable_unsafe_noiommu_mode`
3. Use `uio_pci_generic` instead

**Mellanox device not working.** Verify packages and kernel module:

```bash
apt install rdma-core libibverbs-dev
lsmod | grep mlx5
```
