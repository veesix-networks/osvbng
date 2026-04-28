# NIC Reference

Per-NIC details: hardware specs, driver strategy, throughput limits,
firmware/kernel requirements, and known caveats.

For dataplane modes, vendor support matrix, and bind strategy
explanations, see the [Reference overview](../index.md).

## Intel

- [X710 / XXV710 / XL710 (i40e)](intel-x710.md)
- [E810 (ice)](intel-e810.md)

## Mellanox / NVIDIA

- [ConnectX-4 / 5 / 6 / 7 (mlx5)](mellanox-connectx.md)

## Other

- [Adding vendor support](adding-vendor-support.md): how to extend
  osvbng with a new NIC vendor.
