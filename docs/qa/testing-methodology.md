# Testing & Quality Assurance

osvbng sits in the critical path of subscriber connectivity. A BNG failure means subscribers lose internet access. Every release goes through automated control plane testing, and we validate dataplane performance on real hardware with industry-standard traffic generators.

This page describes what we test, how we test it, and what we plan to improve. If something isn't tested yet, we say so.

## Control Plane Testing

### Automated Integration Tests

The integration suites focus on control plane correctness. They run on a self-hosted runner when changes are merged to main, not on pull requests; a pull request is gated on build and unit tests only. The suites are built from:

- **[Robot Framework](https://robotframework.org/)** for test orchestration
- **[BNG Blaster](https://github.com/rtbrick/bngblaster)** (by RtBrick) for subscriber session simulation and basic traffic verification
- **[containerlab](https://containerlab.dev/)** for deploying full network topologies in Docker
- **[FRRouting](https://frrouting.org/)** for core router simulation (BGP, OSPF)
- **[FreeRADIUS](https://freeradius.org/)** for AAA authentication testing
- **[ISC Kea](https://www.isc.org/kea/)** for DHCP relay/proxy testing
- **[xl2tpd](https://github.com/xelerance/xl2tpd)** for an external L2TPv2 LAC that dials the LNS suite, built from `test-infra/xl2tpd/`

The dataplane the tests run against is the stock VPP image plus the twelve prebuilt osvbng plugins checked in under `test-infra/vpp-plugins/`, which the Docker build copies into `vpp_plugins/`.

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
| 17 | HA Tracker Promotion IPoE | Tracker-driven automatic promotion from STANDBY_ALONE to ACTIVE_SOLO on access interface failure |
| 18 | IPoE Linux Client | Real Linux subscriber with QinQ VLANs, DHCP, ping, and iperf3 throughput |
| 19 | IPoE Linux Client CAKE | Same topology with the CAKE scheduler applied to the session |
| 20 | Routing Policy | Prefix-sets, community-sets, AS-path-sets, and route-policy application to BGP/OSPF |
| 21 | CLI / OpenAPI | Northbound REST API generated from CLI handler registry; schema parity |
| 22 | MSS Clamp | TCP MSS clamping with PPP MRU configuration (RFC 4638) |
| 23 | RADIUS CoA | RADIUS Change-of-Authorization and Disconnect-Message per RFC 5176 |
| 24 | VRF-Lite | End-to-end VRF-lite with subscriber-group VRF cascade onto access sub-interfaces |
| 25 | L3VPN | MPLS L3VPN over LDP and VPNv4: per-VRF pool isolation, RD/RT advertisement, two-label stack |
| 26 | DHCPv6 LDRA | Local DHCPv6 provider terminating RFC 6221 LDRA Relay-Forward from access |
| 27 | API Pagination | Pagination across list-returning northbound show endpoints |
| 28 | Northbound API TLS | Northbound API + Prometheus over TLS with HTTP auth; CGNAT exporter dialer |
| 29 | RADIUS VRF | RADIUS auth/acct sockets pinned to MGMT-VRF with `SO_BINDTODEVICE` proof |
| 30 | L2TP LNS | L2TPv2 LNS terminating an external xl2tpd LAC, PPP negotiated locally |
| 31 | L2TP LAC | L2TPv2 LAC driven by AAA tunnel attributes, PPP bridged to a BNG Blaster LNS |
| 32 | IPoE OpDB Restore | IPoE sessions restored from the operational database across osvbngd, VPP, and container restarts |
| 33 | PPPoE OpDB Restore | Same restart matrix for PPPoE sessions |
| 34 | CGNAT Restart Idempotent | CGNAT mappings preserved across restart with identical and with drifted config |
| 35 | IPoE Family Disabled | Per-group address family gating: v4-only and v6-only groups reject the other family, drop counters increment |
| 36 | AAA Empty Username | Sessions with an empty username are rejected and none establish |
| 37 | HA Graceful Switchover IPoE | Operator-triggered switchover with CGNAT: session restore, GARP flood on access, hitless traffic |
| 38 | L2GW Static | Layer 2 wholesale cross-connects from static circuit config, no local termination |
| 39 | L2GW Dynamic | RADIUS-driven circuits with accounting, Prometheus metrics, restart survival, and CoA teardown |
| 40 | L2GW Packet Trigger | Circuits created on the first PPPoE packet, with restart survival and idle timeout |
| 40 | L2GW VXLAN | Wholesale circuits over static VXLAN tunnel interfaces |
| 41 | Upgrade Tier A v2 | Tier A v2 upgrade pipeline on a QEMU image: apply, rollback, partial-apply guard, stepwise, session survival |
| 42 | IPoE VXLAN PWHE | IPoE sessions terminated on a pseudowire headend over a static VXLAN transport |
| 43 | PPPoE VXLAN PWHE | PPPoE sessions on a pseudowire headend over a static VXLAN transport |
| 44 | LAC VXLAN PWHE | L2TP LAC subscriber arriving over a static VXLAN pseudowire |
| 45 | HA PWHE IPoE | Graceful switchover with IPoE sessions on a pseudowire headend |
| 46 | L2GW EVPN | Wholesale circuits over VXLAN tunnels programmed from EVPN VTEP discovery |
| 47 | IPoE EVPN PWHE | IPoE sessions on a pseudowire headend bound by EVPN-discovered transport |
| 48 | PPPoE EVPN PWHE | PPPoE sessions on a pseudowire headend bound by EVPN-discovered transport |
| 49 | LAC EVPN PWHE | L2TP LAC subscriber arriving over an EVPN-discovered pseudowire |
| 50 | HA EVPN PWHE | Anycast VTEP across both nodes, leaf reroutes on switchover, sessions restored hitless |
| 51 | IPoE HQoS S-VLAN | Hierarchical QoS: per-session schedulers attached to S-VLAN aggregates, share distribution, drain on teardown |
| 52 | PPPoE HQoS S-VLAN | Same hierarchical QoS checks for PPPoE sessions |
| 53 | CGNAT VRF | IPoE in a dual-family VRF and PPPoE in an IPv4-only VRF sharing one inside prefix and one outside pool: mappings keyed by VRF with disjoint blocks, NAT traffic in both VRFs, release under the VRF key |

Number 40 is currently used by two directories on disk, `tests/40-l2gw-packet-trigger` and `tests/40-l2gw-vxlan`; both are listed above.

That is 54 suite directories under `tests/`: 53 Robot suites holding 691 test cases, plus suite 41, which drives five shell scenarios against a QEMU image instead of Robot Framework.

CI does not run all of them. 20 suites are wired into the workflows under `.github/workflows/`, so "all suites pass" is not an automated gate. The rest are run locally with `scripts/run-qa-tests.sh`, which picks up every suite directory that has a matching `.robot` file.

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

FD.io maintains its own independent performance validation through the [CSIT project](https://csit.fd.io/) (Continuous System Integration Testing). CSIT runs fully automated throughput, latency, and regression tests against every VPP release using TRex traffic generators on bare-metal testbeds (Intel Icelake, SapphireRapids, AMD EPYC, Nvidia Grace). Tests cover L2 switching, IPv4/IPv6 routing at up to 2 million FIB entries, NAT, IPsec (including Intel QAT hardware acceleration), VXLan, and container networking via memif. Results are published at [csit.fd.io](https://csit.fd.io/) and performance trends are tracked continuously to catch regressions between releases. A detailed breakdown of their testing methodologies, performance testbed configurations, and hardware diagrams can be found in the [CSIT Report](https://docs.fd.io/csit/master/report/).

This matters because osvbng is primarily a control plane implementation. The actual packet forwarding (IPv4/IPv6 routing, QoS policing, interface handling) is done natively by VPP. osvbng manages subscriber sessions, DHCP, AAA, HA state, and CGNAT pool allocation, but once a session is programmed, packets flow through VPP's forwarding graph at the same speeds CSIT benchmarks. We add a small number of custom VPP plugins (IPoE/PPPoE session encapsulation, CGNAT port block allocation), but the vast majority of the forwarding path is unmodified VPP.

The CSIT results are directly representative of what osvbng achieves for forwarding throughput. Our testing focuses on the areas where we add code on top: session setup/teardown rates, CGNAT translation under load, and ensuring our plugins don't introduce regressions.

### osvbng-Specific Testing

<p align="center">
  <img src="/img/testing_dpdk.png" alt="Dataplane Performance Test Setup" style="max-width: 100%; height: auto;">
</p>

We test osvbng's subscriber-facing forwarding on physical hardware using [Cisco TRex](https://trex-tgn.cisco.com/) as the traffic generator. TRex NICs connect directly to the osvbng server, one on the access side and one on the core side. TRex emulates subscriber sessions and generates bidirectional traffic flows through the BNG with DPDK-accelerated forwarding.

We plan to run these tests on every minor and major release, and at some point introduce them for patch releases. Tests use TRex across three packet sizes plus IMIX:

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
| Integration tests | The 20 suites wired into CI pass on merge to main | Partly (CI) |
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
