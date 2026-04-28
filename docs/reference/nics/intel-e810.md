# Intel E810

Intel's E810 family (Columbiaville). Successor to the X710/XXV710/XL710
generation, runs on the `ice` driver.

## Hardware

| Family | Speeds | Driver |
|---|---|---|
| E810-CQDA2 | 2x100 GbE | ice (Linux), net_ice (DPDK) |
| E810-XXV | 4x25 GbE | ice / net_ice |

PCI vendor ID: `8086`.

## Driver strategy

vfio-pci. Same model as the i40e family.

Requirements:

- IOMMU enabled
- `vfio-pci` kernel module loaded


