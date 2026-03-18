# Testing & Quality Assurance

osvbng sits in the critical path of subscriber connectivity. A BNG failure means subscribers lose internet access. Every release goes through automated control plane testing, and we validate dataplane performance on real hardware with industry-standard traffic generators.

This page describes what we test, how we test it, and what we plan to improve. If something isn't tested yet, we say so.

## Control Plane Testing

### Automated Integration Tests

Every pull request and merge to main triggers a full integration test suite focused on control plane correctness using:

- **[Robot Framework](https://robotframework.org/)** for test orchestration
- **[BNG Blaster](https://github.com/rtbrick/bngblaster)** (by RtBrick) for subscriber session simulation and basic traffic verification
- **[containerlab](https://containerlab.dev/)** for deploying full network topologies in Docker
- **[FRRouting](https://frrouting.org/)** for core router simulation (BGP, OSPF)
- **[FreeRADIUS](https://freeradius.org/)** for AAA authentication testing
- **[ISC Kea](https://www.isc.org/kea/)** for DHCP relay/proxy testing

containerlab and BNG Blaster are primarily control plane testing tools. They verify session establishment, protocol negotiation, HA failover, and basic traffic forwarding. They are not used for dataplane throughput or performance benchmarking (see [Dataplane Performance Testing](#dataplane-performance-testing) below for that).

Each test deploys a complete network: BNG nodes, core routers, subscriber simulators, and where needed, RADIUS and DHCP servers. Unlike our Go unit tests which mock certain interfaces, the integration tests run everything for real.

### Test Suites

| # | Suite | What It Tests |
|---|-------|---------------|
| 01 | Smoke | Single-node startup, basic IPoE session |
| 02 | Smoke HA | Two-node HA election and graceful switchover |
| 03 | IPoE Local Auth | IPoE sessions with local authentication (DHCPv4 + DHCPv6) |
| 04 | PPPoE Local Auth | PPPoE sessions with local authentication (PAP/CHAP) |
| 05 | IPoE RADIUS | IPoE sessions with RADIUS authentication |
| 06 | PPPoE RADIUS | PPPoE sessions with RADIUS authentication |
| 07 | DHCP Relay/Proxy | DHCP relay and proxy mode with ISC Kea |
| 08 | CGNAT IPoE PBA | Carrier-Grade NAT with Port Block Allocation (IPoE) |
| 09 | CGNAT IPoE Deterministic | Deterministic NAT with IPoE |
| 10 | CGNAT PPPoE PBA | CGNAT PBA with PPPoE |
| 11 | CGNAT PPPoE Deterministic | Deterministic NAT with PPPoE |
| 12 | CGNAT HA IPoE | CGNAT + HA graceful switchover with IPoE |
| 13 | CGNAT HA PPPoE | CGNAT + HA graceful switchover with PPPoE |
| 14 | HA Failover IPoE | Hard kill active BNG, verify seamless session restore (IPoE + CGNAT) |
| 15 | HA Failover PPPoE | Hard kill active BNG, verify seamless session restore (PPPoE + CGNAT) |
| 16 | HA Failover RADIUS | RADIUS-assigned pool attribute preserved across failover |

620+ total tests across 16 integration suites.

### What Gets Verified

Every test validates end-to-end behavior, not just "did it start":

- **Session establishment**: DHCP discovery/offer/request/ack, PPPoE PADI/PADO/PADR/PADS, LCP/IPCP/IPv6CP negotiation
- **Dual-stack addressing**: IPv4 pool allocation, IPv6 IANA addresses, IPv6 prefix delegation
- **Authentication**: local auth (SQLite), RADIUS (FreeRADIUS), PAP/CHAP
- **Dataplane programming**: session interfaces created, unnumbered configured, CGNAT mappings programmed
- **Bidirectional traffic**: verified in both directions through the BNG
- **NAT traffic**: CGNAT-aware streams verify translation is working (inside to outside and back)
- **HA election**: priority-based election, virtual MAC programming, BGP route advertisement
- **Session sync**: incremental sync from active to standby, verified via API
- **Seamless failover**: kill the active BNG, verify sessions are restored on the standby with zero subscriber renegotiation
- **AAA attribute preservation**: RADIUS-assigned attributes (pool overrides, service groups) survive failover
- **Routing convergence**: OSPF adjacency, BGP session establishment, route advertisement/withdrawal on failover

### HA Failover Testing

<p align="center">
  <img src="/img/testing_ha.png" alt="HA Failover Test Topology" style="max-width: 100%; height: auto;">
</p>

The failover tests (suites 14-16) validate that subscribers survive a hard BNG failure:

1. Deploy two BNG nodes on a shared L2 access segment (Linux bridge)
2. Establish subscriber sessions with bidirectional traffic on the active BNG
3. Verify session state is synced to the standby
4. `docker kill` the active BNG, simulating a hard failure with no graceful shutdown
5. Wait for the standby to detect peer loss
6. Promote the standby (simulates operator or tracker-driven failover)
7. Verify all sessions are restored from synced state without any subscriber interaction
8. Verify CGNAT mappings are restored with the same outside IP and port block
9. Verify all traffic streams recover and are verified bidirectionally
10. Verify zero session renegotiations: `sessions-flapped: 0`, no new DHCP or PPPoE establishment

The subscriber never notices. Same IP, same NAT mapping, traffic resumes.

### Diagnostics

Every test run captures the last 200 lines of all container logs into the Robot Framework HTML report. When something fails, the logs are immediately available for diagnosis.

All test results, CI logs, and reports are public. Most vendors keep their testing behind paywalls, only share results with their largest customers, or make you chase them for it. We're transparent. Every osvbng test run is publicly visible at the [GitHub repository](https://github.com/veesix-networks/osvbng/actions).

## Dataplane Performance Testing

### Forwarding Engine

osvbng's dataplane is built on [FD.io VPP](https://fd.io/) (Vector Packet Processing), a high-performance forwarding engine already used in production by telecom operators for traditional routing functionality. The FD.io Technical Steering Committee is driven by employees from Cisco Systems, Ericsson, Netgate, Intel, and others.

FD.io maintains its own independent performance validation through the [CSIT project](https://csit.fd.io/) (Continuous System Integration and Testing). CSIT runs fully automated throughput, latency, and regression tests against every VPP release using TRex traffic generators on bare-metal testbeds (Intel Icelake, SapphireRapids, AMD EPYC, Nvidia Grace). Tests cover L2 switching, IPv4/IPv6 routing at up to 2 million FIB entries, NAT, IPsec (including Intel QAT hardware acceleration), VXLan, and container networking via memif. Results are published at [csit.fd.io](https://csit.fd.io/) and performance trends are tracked continuously to catch regressions between releases. A detailed breakdown of their testing methodologies, performance testbed configurations, and hardware diagrams can be found in the [CSIT Report](https://docs.fd.io/csit/master/report/).

This matters because osvbng is primarily a control plane implementation. The actual packet forwarding (IPv4/IPv6 routing, QoS policing, interface handling) is done natively by VPP. osvbng manages subscriber sessions, DHCP, AAA, HA state, and CGNAT pool allocation, but once a session is programmed, packets flow through VPP's forwarding graph at the same speeds CSIT benchmarks. We add a small number of custom VPP plugins (IPoE/PPPoE session encapsulation, CGNAT port block allocation), but the vast majority of the forwarding path is unmodified VPP.

The CSIT results are directly representative of what osvbng achieves for forwarding throughput. Our testing focuses on the areas where we add code on top: session setup/teardown rates, CGNAT translation under load, and ensuring our plugins don't introduce regressions.

### osvbng-Specific Testing

<p align="center">
  <img src="/img/testing_dpdk.png" alt="Dataplane Performance Test Setup" style="max-width: 100%; height: auto;">
</p>

We test osvbng's subscriber-facing forwarding on physical hardware using [Cisco TRex](https://trex-tgn.cisco.com/) as the traffic generator. TRex NICs connect directly to the osvbng server, one on the access side and one on the core side. TRex emulates subscriber sessions and generates bidirectional traffic flows through the BNG with DPDK-accelerated forwarding.

We currently run these tests manually before each release using TRex across three packet sizes plus IMIX:

| Packet Size | Purpose |
|-------------|---------|
| **64 bytes** | Worst-case PPS (smallest packets, highest forwarding demand) |
| **576 bytes** | Typical mixed traffic |
| **1500 bytes** | Maximum throughput (full Ethernet MTU) |
| **IMIX** | Realistic traffic distribution mixing small, medium, and large packets |

### Where We're Heading

We want to automate and formalize our performance testing so that every minor and major release includes published results. The goal is to capture and share with the community:

- Packets per second (PPS) per direction and aggregate, graphed over the test duration
- Throughput (Gbps) per direction and aggregate
- Latency: min, avg, max, P99
- Packet loss verification (must be 0% at rated throughput)
- CPU utilization per worker core
- Memory usage (dataplane buffers, control plane heap)
- Extended soak tests (8 hours baseline, 48 hours for major releases) to catch memory leaks and gradual degradation
- Automated regression detection between releases

We also want to provide these results across various hardware configurations, from small power-efficient 1U servers suited to smaller deployments up to 2U+ high-performance servers for maximum throughput and PPS. Operators should be able to find published numbers for hardware similar to what they plan to deploy.

This is not done yet. Today the performance testing is manual and results are captured as screen captures and log outputs rather than formally published as ingestable data like CSVs or graphs. We're working toward making this a standard part of every release.

## Release Qualification

### What Must Pass Before a Release Ships

| Gate | Description | Automated? |
|------|-------------|------------|
| Build | Binary and Docker image build successfully | Yes (CI) |
| Unit tests | All unit tests pass | Yes (CI) |
| Integration tests | All 16 suites pass | Yes (CI) |
| HA failover | Sessions survive hard BNG failure | Yes (CI) |
| Changelog | Auto-generated via conventional commits | Yes (release-please) |
| Docker image | Published to Docker Hub | Yes (CI) |

### What We're Adding

| Gate | Description | Status |
|------|-------------|--------|
| Automated performance tests | TRex tests with published PPS graphs | In progress |
| Extended soak test | Continuous operation without restart or memory leak | Planned (before v1.0.0) |
| Race detection | `go test -race` on every PR | Planned |
| Coverage tracking | No coverage decrease between releases | Planned |

## Questions?

Join the [Discord community](https://dsc.gg/osvbng) or the [GitHub Discussions](https://github.com/veesix-networks/osvbng/discussions) to discuss testing, report issues, or ask about our QA process.
