# Supported NICs

osvbng supports multiple Dataplane modes:

| Mode | Interface Type | Performance | Notes |
|------|---------------|-------------|-------|
| DPDK | Native DPDK PMD | Line-rate | Vendor-specific binding |
| AF_XDP | eBPF/XDP | High | Coming soon |
| AF_PACKET | Linux kernel | Standard | Any NIC |

This document covers DPDK mode configuration. Different NIC vendors require different driver binding strategies.

## Vendor Support Matrix

| Vendor | Driver Strategy | Dataplane Ownership | Linux Visibility | Notes |
|--------|----------------|---------------|------------------|-------|
| Mellanox/NVIDIA | Bifurcated | Shared | Yes | Uses mlx5 PMD with kernel driver |
| Intel | vfio-pci | Exclusive | No | Requires IOMMU |
| Other | vfio-pci | Exclusive | No | Falls back to vfio-pci |

## Mellanox (ConnectX-4/5/6/7)

**Vendor ID:** `15b3`

**Strategy:** Bifurcated driver

Mellanox NICs use a bifurcated driver architecture where:
- Kernel driver (`mlx5_core`) remains bound
- DPDK `mlx5` PMD accesses hardware via Verbs API
- Data plane is 100% DPDK (kernel bypassed for packets)
- Control plane shared (link status, ethtool, firmware)

**Implications:**
- Interfaces appear in both Linux (`eth1`, `eth2`) and Dataplane
- Linux interfaces should remain unconfigured (no IP)
- Full line-rate DPDK performance
- Works in VMs with PCI passthrough (no guest IOMMU required)

**Requirements:**
- `rdma-core` / `libibverbs` packages
- `mlx5_core` kernel module

## Intel (X710, XXV710, E810, etc.)

**Vendor ID:** `8086`

**Strategy:** vfio-pci

Intel NICs use the standard DPDK model:
- Device unbound from kernel driver (`i40e`, `ice`)
- Bound to `vfio-pci` for DPDK access
- Dataplane has exclusive ownership
- Device disappears from Linux

**Implications:**
- Requires working IOMMU
- Not suitable for VMs without nested IOMMU
- Device invisible to Linux tools

**Requirements:**
- IOMMU enabled (`intel_iommu=on iommu=pt`)
- `vfio-pci` kernel module

## Adding New Vendor Support

NIC vendor implementations are located in `pkg/config/system/nic/`. Each vendor has its own file.

**1. Create a new vendor file** (e.g., `pkg/config/system/nic/broadcom.go`):

```go
package nic

type Broadcom struct{}

func (b Broadcom) Name() string { return "Broadcom" }

func (b Broadcom) Match(vendorID string) bool {
    return vendorID == "14e4"  // PCI vendor ID
}

func (b Broadcom) BindStrategy() BindStrategy {
    return BindStrategyVFIO  // or BindStrategyBifurcated, BindStrategyUIO
}
```

**2. Register the vendor in `pkg/config/system/nic/all.go`:**

```go
func init() {
    Register(Mellanox{})
    Register(Intel{})
    Register(Broadcom{})  // Add before Generic
    Register(Generic{})   // Generic must be last (matches everything)
}
```

**File structure:**
```
pkg/config/system/nic/
├── nic.go       # Vendor interface and registration logic
├── bind.go      # Device binding (vfio-pci, uio, bifurcated)
├── mellanox.go  # Mellanox vendor
├── intel.go     # Intel vendor
├── generic.go   # Generic fallback
└── all.go       # Registration order
```

## Bind Strategies

### BindStrategyBifurcated
- Kernel driver stays bound
- DPDK PMD works alongside kernel
- Used by: Mellanox

### BindStrategyVFIO
- Unbind from kernel, bind to `vfio-pci`
- Requires IOMMU
- Exclusive DPDK ownership
- Used by: Intel, most other vendors

### BindStrategyUIO
- Unbind from kernel, bind to `uio_pci_generic`
- No IOMMU required
- Less secure (no DMA protection)
- Fallback when vfio-pci unavailable

## Troubleshooting

### Device not appearing in Dataplane

Check driver binding:
```bash
ls -la /sys/bus/pci/devices/0000:XX:00.0/driver
```

For Mellanox, should show `mlx5_core`.
For Intel/others with vfio, should show `vfio-pci`.

### vfio-pci bind fails with EINVAL (-22)

IOMMU not available. Options:
1. Enable IOMMU in BIOS and kernel (`intel_iommu=on`)
2. Use noiommu mode (less secure): `echo 1 > /sys/module/vfio/parameters/enable_unsafe_noiommu_mode`
3. Use `uio_pci_generic` instead

### Mellanox device not working

Ensure required packages are installed:
```bash
apt install rdma-core libibverbs-dev
```

Check kernel module:
```bash
lsmod | grep mlx5
```
