# Validating New Hardware

How to add a server to the [Hardware Matrix](hardware-matrix.md).

> A full automated integration test that runs end-to-end against new
> hardware (subscriber establishment, throughput, QoS, CGNAT) is
> being tracked in
> [veesix-networks/osvbng#297](https://github.com/veesix-networks/osvbng/issues/297).
> The steps below are the manual process used until that lands.

## 1. Document the design

Copy [`hardware-designs/_template.md`](hardware-designs/_template.md)
to `hardware-designs/<vendor>-<model>.md`. Fill in:

- Bill of materials (chassis, CPU, RAM, NICs, storage, PSU)
- CPU layout (sockets, cores/threads, NUMA topology, planned VPP
  pinning)
- Network layout (which port serves which role, link speed)
- Software under test (osvbng version, VPP version, kernel, DPDK
  driver mode)

## 2. Run the validation harness

The test profile and result fields are defined by the automated
integration harness tracked in
[veesix-networks/osvbng#297](https://github.com/veesix-networks/osvbng/issues/297).
This section will be filled in once the harness lands. Until then,
record results captured from the existing QA suite on the design page.

## 3. Add to the matrix

Add a row to [Hardware Matrix](hardware-matrix.md). The model name in
the first column should link to your design page.

## 4. (Optional) Add to the sidebar

If the design is significant enough to surface in the left nav, add
its design page to `mkdocs.yml`. Otherwise the matrix link is enough.
