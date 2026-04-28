# HP ProLiant DL360 Gen10

## Specification

Chassis PID: **867961-B21**

| Component | Spec |
|---|---|
| Chassis | HPE ProLiant DL360 Gen10, 1U |
| CPU | 2x Intel Xeon Gold 5120 (14c/28t @ 2.2 GHz) |
| RAM | 192 GB DDR4 2400 MHz ECC |
| Boot storage | 240 GB Crucial SSD |
| Management NIC | Broadcom BCM5719 4x1 GbE (onboard) |
| Dataplane NIC | Mellanox ConnectX-5 2x100 GbE |
| OOB management | HPE iLO 5 |
| Power | 2x 800W redundant (HPE 720479-B21) |

Indicative cost: £987 ex-VAT (£1,184.40 inc. 20% VAT), used/refurb.

## CPU layout

- Sockets: 2
- Cores per socket: 14 (HT on, 2 threads/core)
- Logical CPUs total: 56
- NUMA nodes: 2
- Pinning:

## Network layout

| Interface | Hardware | Role | Speed |
|---|---|---|---|
| eth0..3 | BCM5719 | management | 4x 1 GbE |
|  | ConnectX-5 port 0 | dataplane |  |
|  | ConnectX-5 port 1 | dataplane |  |

## Software under test

- osvbng version: 
- VPP version: 
- Linux distribution / kernel: 
- DPDK driver mode: bifurcated (mlx5)

## Test profile

Filled in by the validation harness ([osvbng#297](https://github.com/veesix-networks/osvbng/issues/297)).

## Results

| Metric | Result | Notes |
|---|---|---|
| Max subscribers (dual-stack) |  | |
| Max throughput |  | |
| Throughput with QoS |  | |
| CGNAT sessions |  | |
| Notable bottleneck |  | |

## NIC reference

- [Mellanox / NVIDIA ConnectX](../nics/mellanox-connectx.md)

## Test date

- 
- Tested by: 
