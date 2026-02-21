# Changelog

## [0.2.0](https://github.com/veesix-networks/osvbng/compare/v0.1.2...v0.2.0) (2026-02-21)


### Features

* **aaa:** add policy-based authentication mode ([#124](https://github.com/veesix-networks/osvbng/issues/124)) ([8a1758e](https://github.com/veesix-networks/osvbng/commit/8a1758e3aaca3a6a9ce21553eb249e5dc849f8c5))
* **aaa:** add pool and service group attribute mappings ([#109](https://github.com/veesix-networks/osvbng/issues/109)) ([75237b8](https://github.com/veesix-networks/osvbng/commit/75237b8dd3be13dfdf126040282212b2f945c4a3))
* **aaa:** log returned attributes in authentication response ([#116](https://github.com/veesix-networks/osvbng/issues/116)) ([dbdd00d](https://github.com/veesix-networks/osvbng/commit/dbdd00dce7139b09ae8f72a30723aeb7a6731671))
* **bgp:** add VPNv4/VPNv6 address family config model and templates ([#97](https://github.com/veesix-networks/osvbng/issues/97)) ([8ecbeb9](https://github.com/veesix-networks/osvbng/commit/8ecbeb9fe598893fa21446c8d0eef9d3d7cfda6b))
* **bgp:** add VPNv4/VPNv6 and L3VPN configuration and show handlers ([#98](https://github.com/veesix-networks/osvbng/issues/98)) ([958a7fe](https://github.com/veesix-networks/osvbng/commit/958a7fe7ab144be76c904a9c181728f88403696e))
* **dataplane:** add LCP namespace support with routing protocol fixes ([#99](https://github.com/veesix-networks/osvbng/issues/99)) ([9823ae7](https://github.com/veesix-networks/osvbng/commit/9823ae7faac8eba3cec98c6f2693e9262a68a2e5))
* **dhcp:** add DHCP profile types and shared allocator ([#106](https://github.com/veesix-networks/osvbng/issues/106)) ([82e8bb1](https://github.com/veesix-networks/osvbng/commit/82e8bb1d89a9d2a31d67ed7f81ead3c777256f6b))
* **dhcp:** add per-VRF pool isolation to allocator registry ([#112](https://github.com/veesix-networks/osvbng/issues/112)) ([d5a7281](https://github.com/veesix-networks/osvbng/commit/d5a728177f8e8e8d1d9d0ade1adbd5abe2db8787))
* **dhcp:** add typed AAA attributes and wire DHCPv4 provisioning context ([#107](https://github.com/veesix-networks/osvbng/issues/107)) ([5be6049](https://github.com/veesix-networks/osvbng/commit/5be60490e256e960abf977a8689555e0c1c77eef))
* **dhcp:** add VRF-aware pool overflow for IPv4, IANA, and PD ([#118](https://github.com/veesix-networks/osvbng/issues/118)) ([2652cf0](https://github.com/veesix-networks/osvbng/commit/2652cf06be6bc641ba196bbe5305637067c82a1e))
* **dhcp:** centralize IP allocation in resolve layer ([#110](https://github.com/veesix-networks/osvbng/issues/110)) ([597e77b](https://github.com/veesix-networks/osvbng/commit/597e77b9e5294874a33cbe428f8532e0d5d3d316))
* **dhcpv6:** wire provisioning context through DHCPv6 provider ([#108](https://github.com/veesix-networks/osvbng/issues/108)) ([c7ef3a6](https://github.com/veesix-networks/osvbng/commit/c7ef3a6f1312d0481d19342b3f874d24a395ba91))
* **ifmgr:** track interface IP addresses and FIB table IDs ([#93](https://github.com/veesix-networks/osvbng/issues/93)) ([8cfc5f2](https://github.com/veesix-networks/osvbng/commit/8cfc5f2d2176d296b4813dff5af2f88381f3a653))
* **l3vpn:** add L3VPN dev environment with loopback-based peering ([#103](https://github.com/veesix-networks/osvbng/issues/103)) ([816f2b1](https://github.com/veesix-networks/osvbng/commit/816f2b1aed83dc476678dc62d4c85743bda5e7c9))
* **mpls:** add MPLS/LDP southbound API, config model, and FRR templates ([#96](https://github.com/veesix-networks/osvbng/issues/96)) ([5f314a0](https://github.com/veesix-networks/osvbng/commit/5f314a09be790eef7339258003ff3025168e4796))
* **qos:** implement per-subscriber policer lifecycle ([#120](https://github.com/veesix-networks/osvbng/issues/120)) ([1b6f6ca](https://github.com/veesix-networks/osvbng/commit/1b6f6caa39274084b7d4047e49ab59a214ca92a2))
* **routing:** add VRF assignment to IPoE and PPPoE subscriber sessions ([bbbb6b7](https://github.com/veesix-networks/osvbng/commit/bbbb6b789ecf9d2ec218210ce392d531174217d6))
* **routing:** add VRF manager with Linux VRF and VPP table lifecycle ([1334211](https://github.com/veesix-networks/osvbng/commit/133421170a3995f1500cb0cb60a79b4956d0f7fc))
* **routing:** add VRF manager with Linux VRF and VPP table lifecycle ([#89](https://github.com/veesix-networks/osvbng/issues/89)) ([6c43cfe](https://github.com/veesix-networks/osvbng/commit/6c43cfe476e55ad73425410efd0e764a37e44b03))
* **routing:** bind infrastructure interfaces to VRF during creation ([3b838cd](https://github.com/veesix-networks/osvbng/commit/3b838cd332210d93445ebaf2a34e5d2cb838e688))
* **routing:** wire VRF manager into application startup and config loading ([c6b9546](https://github.com/veesix-networks/osvbng/commit/c6b95467b268dab3951765415f1045da9bd98002))
* **svcgroup:** add service group resolver with three-layer merge resolution ([33f679f](https://github.com/veesix-networks/osvbng/commit/33f679fb00fed4f76409d51f21ff2ebdf81d75c6))
* **svcgroup:** added support for service groups ([aa02eb8](https://github.com/veesix-networks/osvbng/commit/aa02eb8281ab051a3413f0401646fa7cdf7113de))
* **svcgroup:** added support for service groups ([#91](https://github.com/veesix-networks/osvbng/issues/91)) ([aa02eb8](https://github.com/veesix-networks/osvbng/commit/aa02eb8281ab051a3413f0401646fa7cdf7113de))


### Bug Fixes

* **arp:** enforce VRF-aware ARP response filtering ([#94](https://github.com/veesix-networks/osvbng/issues/94)) ([bf7bb78](https://github.com/veesix-networks/osvbng/commit/bf7bb78f91415089541b9f0f2d4a01fac2a0cfbe))
* **arp:** use per-interface IP dump and ifmgr cache ([#95](https://github.com/veesix-networks/osvbng/issues/95)) ([65dea39](https://github.com/veesix-networks/osvbng/commit/65dea39741bf490da6e43bf31b27d6ec9250385c))
* **bgp:** activate neighbors in unicast address families ([#121](https://github.com/veesix-networks/osvbng/issues/121)) ([2187724](https://github.com/veesix-networks/osvbng/commit/2187724bcb4e06d6bfa793f824a836d1a02ff768))
* **bgp:** add blackhole routes for advertised pool networks ([#122](https://github.com/veesix-networks/osvbng/issues/122)) ([8c17fdf](https://github.com/veesix-networks/osvbng/commit/8c17fdf60dac1c60ca46f0e2e70b9adc8caec6d3))
* **bgp:** add no bgp default ipv4-unicast to template ([#101](https://github.com/veesix-networks/osvbng/issues/101)) ([9574dcf](https://github.com/veesix-networks/osvbng/commit/9574dcfbf0428da99822b5ac54454c5e81ae1878))
* **config:** stabilize topological sort for deterministic change ordering ([5074cb3](https://github.com/veesix-networks/osvbng/commit/5074cb361e23518c8a0fd89fffe20e3f8eae2b05))
* **dataplane:** bring up loopback in LCP namespace ([#102](https://github.com/veesix-networks/osvbng/issues/102)) ([a75cce3](https://github.com/veesix-networks/osvbng/commit/a75cce3b5531f1db86e4df92f8dd578e1c1ed6c5))
* **dataplane:** bring up loopback in LCP namespace and register in ifmgr ([#104](https://github.com/veesix-networks/osvbng/issues/104)) ([7736aee](https://github.com/veesix-networks/osvbng/commit/7736aee81fd91db060189e96adf528b279c19a08))
* **dhcp:** detect and reject static/dynamic IP reservation collisions ([#119](https://github.com/veesix-networks/osvbng/issues/119)) ([2b69092](https://github.com/veesix-networks/osvbng/commit/2b6909219136a457092a91da87b7994e9d849fe2))
* **dhcp:** resolve per-pool gateway and add service group pool selection ([#117](https://github.com/veesix-networks/osvbng/issues/117)) ([4dcde9e](https://github.com/veesix-networks/osvbng/commit/4dcde9eaa4f882b5e4aedab16482bdc5f2844581))
* **ipoe:** log error when address profile not found ([#125](https://github.com/veesix-networks/osvbng/issues/125)) ([78c6f5e](https://github.com/veesix-networks/osvbng/commit/78c6f5e1fb69e81e25356f17fbba60ce6fc3d8d8))
* **ospf:** use accept-all-interfaces mfib flag ([#100](https://github.com/veesix-networks/osvbng/issues/100)) ([9fa851e](https://github.com/veesix-networks/osvbng/commit/9fa851e12039289c872fa27146c0ee979efc74f6))
* **southbound:** hardcode LCP dataplane namespace and handle existing pairs on restart ([#127](https://github.com/veesix-networks/osvbng/issues/127)) ([fb884a6](https://github.com/veesix-networks/osvbng/commit/fb884a6c422f0b1323632041829db307b67c4a18))
* **southbound:** set af-packet MAC at creation instead of post-hoc sync ([#123](https://github.com/veesix-networks/osvbng/issues/123)) ([43b0b69](https://github.com/veesix-networks/osvbng/commit/43b0b69003bd19dc4b9c419fe82dff88dd2fc1f6))

## [0.1.2](https://github.com/veesix-networks/osvbng/compare/v0.1.1...v0.1.2) (2026-02-13)


### Bug Fixes

* **dataplane:** default QinQ outer TPID to 802.1ad ([008e63c](https://github.com/veesix-networks/osvbng/commit/008e63c7fa57bb9128f4c26dd0c70048ad77559b))
* **dataplane:** default QinQ outer TPID to 802.1ad ([#86](https://github.com/veesix-networks/osvbng/issues/86)) ([008e63c](https://github.com/veesix-networks/osvbng/commit/008e63c7fa57bb9128f4c26dd0c70048ad77559b))
* **dataplane:** default QinQ outer TPID to 802.1ad with per-group override ([691090a](https://github.com/veesix-networks/osvbng/commit/691090ab60b888ff87f633be019d02692d506658))
* **routing:** use loaded config for FRR config generation ([680f559](https://github.com/veesix-networks/osvbng/commit/680f559bb16794569c0751bc9e773385a0ce22f9))
* **routing:** use loaded config for FRR config generation ([5fe6dac](https://github.com/veesix-networks/osvbng/commit/5fe6daca8a38cf6016a711246cf99bfedfa654c5))
* **routing:** use loaded config for FRR config generation ([#88](https://github.com/veesix-networks/osvbng/issues/88)) ([680f559](https://github.com/veesix-networks/osvbng/commit/680f559bb16794569c0751bc9e773385a0ce22f9))

## [0.1.1](https://github.com/veesix-networks/osvbng/compare/v0.1.0...v0.1.1) (2026-02-10)


### Bug Fixes

* **build:** copy template subdirectories in qemu image build ([f003ae7](https://github.com/veesix-networks/osvbng/commit/f003ae7c003543f56663df2d8c22129d8ea795a0))
* **build:** copy template subdirectories in qemu image build ([#81](https://github.com/veesix-networks/osvbng/issues/81)) ([e4e9410](https://github.com/veesix-networks/osvbng/commit/e4e9410fc5960085453fba74d396a52a4f9c3020))
* **ipoe:** reset stale AAA-approved sessions ([#82](https://github.com/veesix-networks/osvbng/issues/82)) ([3d26c2e](https://github.com/veesix-networks/osvbng/commit/3d26c2e720fec2c939b88fadc2f4c539b747ca16))

## [0.1.0](https://github.com/veesix-networks/osvbng/compare/v0.0.4...v0.1.0) (2026-02-10)


### Features

* **ipoe:** punt IPv6 RS to control plane for per-subscriber RA handling ([#73](https://github.com/veesix-networks/osvbng/issues/73)) ([8fe8956](https://github.com/veesix-networks/osvbng/commit/8fe89567952c847b1ca789c837b5630c844ee2fe))
* **models:** add username to subscriber session model ([#76](https://github.com/veesix-networks/osvbng/issues/76)) ([718c3b0](https://github.com/veesix-networks/osvbng/commit/718c3b02ae05b3bbdf48a204dcef451e3b8b4eb9))
* **monitoring:** add subscriber session prometheus metrics and grafana dashboard ([#78](https://github.com/veesix-networks/osvbng/issues/78)) ([cb5f1b6](https://github.com/veesix-networks/osvbng/commit/cb5f1b6e7d32b4227ee191a9e6bd87a281a9cae6))
* **subscriber:** subscriber clear session functionality ([#77](https://github.com/veesix-networks/osvbng/issues/77)) ([854beff](https://github.com/veesix-networks/osvbng/commit/854beff21b4ebff4068686208ed489652304cda8))


### Bug Fixes

* **pppoe:** resolve PPPoE session egress and unicast packet handling ([#75](https://github.com/veesix-networks/osvbng/issues/75)) ([4533c25](https://github.com/veesix-networks/osvbng/commit/4533c25b86a394718eb7e1ca50f6a3f53e479917))
* **subscriber:** count dual-stack sessions by address presence and fix in-memory cache scan ([#79](https://github.com/veesix-networks/osvbng/issues/79)) ([9a806f0](https://github.com/veesix-networks/osvbng/commit/9a806f019b0141785472050a48b01c6a58330951))
