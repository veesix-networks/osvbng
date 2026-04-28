# Hardware Matrix

Validated reference designs for osvbng. Each row links to a design
page with the full specification, test profile, and results.

> Numbers are not vendor-quoted line rate. They are achieved figures
> from osvbng's own QA suite (or operator submissions). The
> automated harness that produces them is tracked in
> [osvbng#297](https://github.com/veesix-networks/osvbng/issues/297).

## Validated designs

| Vendor / Model | Max subscribers (dual-stack) | Max throughput | Throughput w/ QoS | CGNAT sessions | Indicative cost |
|---|---|---|---|---|---|
| [HP ProLiant DL360 Gen10](hardware-designs/hp-dl360-gen10.md) | | | | | £987 ex-VAT (used) |

## Adding a server

See [Validating new hardware](validating-hardware.md).

## NIC reference

For NIC-specific limits (small-packet ceilings, IOMMU requirements,
driver strategy) see [NIC reference](nics/index.md).
