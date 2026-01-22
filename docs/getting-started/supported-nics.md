# Supported NICs

osvbng supports multiple dataplane modes to suit different deployment scenarios.

## Dataplane Modes

| Mode | Performance | Use Case | NIC Requirements |
|------|-------------|----------|------------------|
| **DPDK** | Line-rate | Production, high-throughput | Supported NICs (see below) |
| **AF_XDP** | High | Production, simpler setup | Any XDP-capable NIC (coming soon) |
| **AF_PACKET** | Standard | Development, any NIC | Any Linux NIC |

## DPDK Mode

For high-throughput production deployments, DPDK provides line-rate performance.

### Supported NICs for DPDK

| Vendor | Supported Models | Driver Strategy |
|--------|-----------------|-----------------|
| **Mellanox/NVIDIA** | ConnectX-4, ConnectX-5, ConnectX-6, ConnectX-7 | Bifurcated |
| **Intel** | X710, XXV710, E810 series | vfio-pci |

### Mellanox/NVIDIA (Recommended)

Mellanox ConnectX series NICs are recommended for production:

- Full line-rate performance
- Works seamlessly in VMs with PCI passthrough
- No IOMMU configuration required in guest

```yaml
dataplane:
  dpdk:
    devices:
      - pci: "0000:05:00.0"
        name: eth1
      - pci: "0000:06:00.0"
        name: eth2
```

### Intel

Intel NICs require IOMMU:

- Requires `intel_iommu=on iommu=pt` kernel parameters
- May require additional configuration in VM environments

## AF_XDP Mode (Coming Soon)

AF_XDP provides high performance using eBPF/XDP while keeping interfaces visible in Linux. Ideal for deployments where DPDK driver binding is not practical.

## AF_PACKET Mode

Works with any Linux NIC. Suitable for development and testing.

## Next Steps

For detailed technical information about NIC driver strategies and vendor support, see [Low Level - Supported NICs](../architecture/SUPPORTED_NICS.md).
