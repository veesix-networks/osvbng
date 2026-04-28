# Intel X710 / XXV710 / XL710

Intel's i40e-family NICs (X710, XXV710, XL710). Common silicon, common
driver, common limits. The newer E810 family is documented separately.

## Hardware

| Family | Speeds | Driver |
|---|---|---|
| X710 | 4x10 GbE | i40e (Linux), net_i40e (DPDK) |
| XXV710 | 2x25 GbE | i40e / net_i40e |
| XL710 | 1x40 GbE or 4x10 GbE | i40e / net_i40e |

PCI vendor ID: `8086`.

## Driver strategy

vfio-pci. Device is unbound from `i40e` and bound to `vfio-pci` for
DPDK. Once bound the device disappears from Linux.

Requirements:

- IOMMU enabled (`intel_iommu=on iommu=pt` on the kernel command line)
- `vfio-pci` kernel module loaded

Not suitable for VMs without nested IOMMU.

## Throughput limits

The whole device shares a packet-processing path with hardware ceilings
that operators must size for. These come from Intel's own datasheets.

### Small packets (<160 B)

Hardware ceiling for the whole device: **~37 Mpps**, regardless of how
many ports are active.

This bounds:

- X710 and XL710 (4x10 GbE mode) when 3 or 4 ports are in use at 10 GbE
- XXV710
- XL710 in 1x40 GbE mode

### Large packets (≥160 B)

- XXV710 (2x25 GbE): total hardware ceiling ~96–97% of dual-port 25 GbE
  line rate per direction. Example: ~45.5 Gb/s per direction for IPv4
  TCP packets >1518 B.

## Caveats

- Requires `intel_iommu=on iommu=pt`.
- Device is invisible to Linux after vfio-pci bind. Use the dataplane
  CLI / vppctl for state inspection.
- Firmware version matters for some PMD features. Match firmware to
  the DPDK version in use.
