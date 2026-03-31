# Changelog

## [0.7.0](https://github.com/veesix-networks/osvbng/compare/v0.6.1...v0.7.0) (2026-03-31)


### Features

* **component:** add readiness signaling for async plugin startup ([#221](https://github.com/veesix-networks/osvbng/issues/221)) ([920bee1](https://github.com/veesix-networks/osvbng/commit/920bee19f9e02d9b662975b13fbf9ef3eb83e8cf))
* **dataplane:** cgroup-aware CPU detection with conservative defaults ([#237](https://github.com/veesix-networks/osvbng/issues/237)) ([5ab23e2](https://github.com/veesix-networks/osvbng/commit/5ab23e215c72aea57e745efcb709d70199b18c01))
* **ha:** add GARP flood on SRG promotion with batching and rate limiting ([#225](https://github.com/veesix-networks/osvbng/issues/225)) ([f010a8b](https://github.com/veesix-networks/osvbng/commit/f010a8bc285a219c50b67ade0b2f1b5deb101205))
* **logger:** async zerolog migration for non-blocking logging ([#217](https://github.com/veesix-networks/osvbng/issues/217)) ([000a879](https://github.com/veesix-networks/osvbng/commit/000a87976eb2a98a7b98105358a5f540603925f1))


### Bug Fixes

* **ci:** add topology cleanup and diagnostics to test workflows ([#234](https://github.com/veesix-networks/osvbng/issues/234)) ([bb71adf](https://github.com/veesix-networks/osvbng/commit/bb71adf824ff2f765c8f779e4ad05abf7f4d6638))
* **ci:** handle non-zero exit codes in test setup version checks ([#235](https://github.com/veesix-networks/osvbng/issues/235)) ([00a2702](https://github.com/veesix-networks/osvbng/commit/00a2702709a0c42fe946284f9af1da73089d18b0))
* **ci:** pre-create containerlab network to prevent parallel deploy race ([#236](https://github.com/veesix-networks/osvbng/issues/236)) ([2303393](https://github.com/veesix-networks/osvbng/commit/23033932034dc04439412b2dc93896bc1d143cd7))
* **dataplane:** default to exactly 1 thread worker regardless of available cores ([#238](https://github.com/veesix-networks/osvbng/issues/238)) ([ed73678](https://github.com/veesix-networks/osvbng/commit/ed736780dd43677d9317d604307941d2fe72b1b8))
* **dataplane:** default VPP heap to 1G and increase egress channel capacity for DPDK scale ([#250](https://github.com/veesix-networks/osvbng/issues/250)) ([8cc78e5](https://github.com/veesix-networks/osvbng/commit/8cc78e56a917937bcf926d3408e8b0b8f0b1a309))
* **dhcpv6:** resolve session cache race between concurrent IPv4 and IPv6 lifecycle events ([#222](https://github.com/veesix-networks/osvbng/issues/222)) ([62c06a8](https://github.com/veesix-networks/osvbng/commit/62c06a89ad724836e26f0b006d64eed551b0ee55))
* **ha:** prevent standby from responding to PPPoE discovery ([#227](https://github.com/veesix-networks/osvbng/issues/227)) ([4d7c570](https://github.com/veesix-networks/osvbng/commit/4d7c5702366c7d83958bbf985966461dfb538115))
* **ha:** prevent standby from responding to subscriber ARP and IPv6 ND ([#226](https://github.com/veesix-networks/osvbng/issues/226)) ([89c84f4](https://github.com/veesix-networks/osvbng/commit/89c84f4f5086732a2776c93f94a4c8ba4a95e8d0))
* **ha:** update GetVirtualMAC test to reflect active-state guard ([#229](https://github.com/veesix-networks/osvbng/issues/229)) ([01a5024](https://github.com/veesix-networks/osvbng/commit/01a50243fdd9617edee377922a8538951fe9fb71))
* **ipoe:** eliminate session index race causing 1-in-48k stuck session ([#248](https://github.com/veesix-networks/osvbng/issues/248)) ([f79dd45](https://github.com/veesix-networks/osvbng/commit/f79dd45ec2b5ecda389c93fb8c84e2f1039a5b24))
* **southbound:** replace FIFO async worker with stream pool and parameterize VPP memory config ([#246](https://github.com/veesix-networks/osvbng/issues/246)) ([b51a6be](https://github.com/veesix-networks/osvbng/commit/b51a6be721ee19ed3e36ea556105974f78b9cd2a))


### Performance Improvements

* **dhcp:** replace gopacket with binary parsers, fix dataplane config generation ([#216](https://github.com/veesix-networks/osvbng/issues/216)) ([c68e95a](https://github.com/veesix-networks/osvbng/commit/c68e95a8afd7d88e07c96643eb16ccd30c5228dd))
* **dhcpv6:** revert IPoE mutex additions, use lightweight subscriber cache merge ([#224](https://github.com/veesix-networks/osvbng/issues/224)) ([5da7514](https://github.com/veesix-networks/osvbng/commit/5da751447ac8ad2bab37eabd4c1b688e0c7e983b))
* **ipoe:** fix DHCPv6 scale bottleneck at 32k dual-stack subscribers ([#244](https://github.com/veesix-networks/osvbng/issues/244)) ([abf37ed](https://github.com/veesix-networks/osvbng/commit/abf37edeec1ceb5bfe03ad84043a0f4d91fe3b87))
* **sessions:** improve IPoE session setup throughput ([#213](https://github.com/veesix-networks/osvbng/issues/213)) ([0a0b97d](https://github.com/veesix-networks/osvbng/commit/0a0b97d36686fdc5850f528f0d6022f2db190155))
* **sessions:** profile-guided session setup optimizations ([#215](https://github.com/veesix-networks/osvbng/issues/215)) ([20fa869](https://github.com/veesix-networks/osvbng/commit/20fa8690b75e17eb3d95c356e96387b9521a34a1))

## [0.6.1](https://github.com/veesix-networks/osvbng/compare/v0.6.0...v0.6.1) (2026-03-22)


### Bug Fixes

* **ci:** trigger discord webhook on release-please PR creation ([#212](https://github.com/veesix-networks/osvbng/issues/212)) ([81299b3](https://github.com/veesix-networks/osvbng/commit/81299b37649682ea6b5b4a18ea887b3f5ed2640d))
* **dataplane:** default AF_PACKET interfaces to interrupt rx-mode ([#209](https://github.com/veesix-networks/osvbng/issues/209)) ([47617ec](https://github.com/veesix-networks/osvbng/commit/47617ece50aa18e63a77aa2a70ea3e7edc7bbc5d))
* **dataplane:** use poll instead of busy-read on punt eventfd ([#211](https://github.com/veesix-networks/osvbng/issues/211)) ([55be33a](https://github.com/veesix-networks/osvbng/commit/55be33aa5daec67996c88672eb4d431d39b4c1ab))

## [0.6.0](https://github.com/veesix-networks/osvbng/compare/v0.5.0...v0.6.0) (2026-03-21)


### Features

* **cgnat:** add Carrier-Grade NAT with PBA mode for IPoE and PPPoE subscribers ([#183](https://github.com/veesix-networks/osvbng/issues/183)) ([c96cee1](https://github.com/veesix-networks/osvbng/commit/c96cee1fb449dd98188a41baf399e4725a5b0e3a))
* **cgnat:** add CGNAT HA mapping sync with incremental and bulk sync ([#188](https://github.com/veesix-networks/osvbng/issues/188)) ([95088e4](https://github.com/veesix-networks/osvbng/commit/95088e496284216dad406c6ad81b6fb30c7df26e))
* **ha:** add tracker-driven promotion from STANDBY_ALONE ([#196](https://github.com/veesix-networks/osvbng/issues/196)) ([7a8222c](https://github.com/veesix-networks/osvbng/commit/7a8222c26e2436bfab3783bc147f029e7be02f5f))
* **ha:** restore synced sessions on HA promotion with failover tests ([#190](https://github.com/veesix-networks/osvbng/issues/190)) ([3bc1dac](https://github.com/veesix-networks/osvbng/commit/3bc1dac5debaa3b0ab5f8894a1eb50361068d2fa))
* **ha:** sync AAA attributes across HA failover with RADIUS validation ([#192](https://github.com/veesix-networks/osvbng/issues/192)) ([504c88b](https://github.com/veesix-networks/osvbng/commit/504c88b4dfbd07f011fa5f3ed4361a99fd9db597))
* **qos:** integrate CAKE scheduler plugin into subscriber lifecycle ([#206](https://github.com/veesix-networks/osvbng/issues/206)) ([dd387a5](https://github.com/veesix-networks/osvbng/commit/dd387a55bf80e1ac9ffbb625cb8be3464b7f7d5e))


### Bug Fixes

* **arp:** ignore DAD probe for client's own assigned IP ([#205](https://github.com/veesix-networks/osvbng/issues/205)) ([678f6a0](https://github.com/veesix-networks/osvbng/commit/678f6a00d50ccb5170394b423bf15c5497c8242a))
* **ci:** add checkout step for Discord changelog notification ([#193](https://github.com/veesix-networks/osvbng/issues/193)) ([ab600b5](https://github.com/veesix-networks/osvbng/commit/ab600b5b89f44fd32c81ba990e398eb316e3dd07))
* **ci:** extract PR number from release-please JSON output ([#199](https://github.com/veesix-networks/osvbng/issues/199)) ([e4a1d95](https://github.com/veesix-networks/osvbng/commit/e4a1d9545cd5e8e23d5424afbf7cc5cff7ba631b))
* **ci:** pass all github expressions as env vars to avoid shell parsing errors ([#195](https://github.com/veesix-networks/osvbng/issues/195)) ([1f7f7b4](https://github.com/veesix-networks/osvbng/commit/1f7f7b4631a466a19dc9915ff951ace21c6fe869))
* **ci:** prevent shell parsing failures in Discord webhook notifications ([#197](https://github.com/veesix-networks/osvbng/issues/197)) ([c388837](https://github.com/veesix-networks/osvbng/commit/c388837aa76674e31b5c78440e5e97b727964021))
* **ci:** use github context instead of git log for Discord notifications ([#194](https://github.com/veesix-networks/osvbng/issues/194)) ([3f03898](https://github.com/veesix-networks/osvbng/commit/3f03898c1f4580682c39d77189eec23554e78bc7))
* **ha:** handle reverse event ordering for tracker promotion ([#198](https://github.com/veesix-networks/osvbng/issues/198)) ([a6071bd](https://github.com/veesix-networks/osvbng/commit/a6071bdac531c3725bd5f73d199429b9170e0694))
* **ha:** only restore synced sessions when promoted from STANDBY_ALONE ([#201](https://github.com/veesix-networks/osvbng/issues/201)) ([2189aee](https://github.com/veesix-networks/osvbng/commit/2189aee5e4afb8770a9726a269f93506776d8f92))

## [0.5.0](https://github.com/veesix-networks/osvbng/compare/v0.4.0...v0.5.0) (2026-03-14)


### Features

* **aaa:** add RADIUS auth provider with server failover and accounting ([#169](https://github.com/veesix-networks/osvbng/issues/169)) ([ad464a3](https://github.com/veesix-networks/osvbng/commit/ad464a37f64e8494ee2de3feaa55750addc3dde7))
* **dhcp:** add relay and proxy providers with Kea dev environment, smoke tests, and CI integration ([#172](https://github.com/veesix-networks/osvbng/issues/172)) ([5b6b794](https://github.com/veesix-networks/osvbng/commit/5b6b794529c0a7834ae3d4c43e39bc4f4a13c66c))


### Bug Fixes

* **aaa:** add Message-Authenticator (attr 80) to Access-Request packets ([#181](https://github.com/veesix-networks/osvbng/issues/181)) ([3f5796e](https://github.com/veesix-networks/osvbng/commit/3f5796e4db8c4c79d28a3ac4791cc72424430c8f))
* **aaa:** address RADIUS auth/accounting issues from code review ([#174](https://github.com/veesix-networks/osvbng/issues/174)) ([28625ae](https://github.com/veesix-networks/osvbng/commit/28625aeef5d19b7a22376efb2138f7c98aa42545))
* **aaa:** use atomic pointer for global RADIUS provider reference ([#180](https://github.com/veesix-networks/osvbng/issues/180)) ([612ff45](https://github.com/veesix-networks/osvbng/commit/612ff455b7e6ec4279c89fa954840b9de35bd7a2))
* **aaa:** wire up RadiusAttr name resolution for response mappings ([#179](https://github.com/veesix-networks/osvbng/issues/179)) ([c84b64e](https://github.com/veesix-networks/osvbng/commit/c84b64e1af4fda0d7152e22da4241b8d2d2b80c3))
* **dhcp:** address relay and proxy issues ([#175](https://github.com/veesix-networks/osvbng/issues/175)) ([a24133d](https://github.com/veesix-networks/osvbng/commit/a24133d1c29da9f6d7873bbccb4209c1613f41b7))
* **dhcp:** use compound keys for pending map to prevent XID collisions ([#178](https://github.com/veesix-networks/osvbng/issues/178)) ([82366cf](https://github.com/veesix-networks/osvbng/commit/82366cf3f505ac78926420fb4bb51461e32a2630))
* **dhcpv6:** add enterprise-number prefix to Remote-ID option per RFC 4649 ([#177](https://github.com/veesix-networks/osvbng/issues/177)) ([b914624](https://github.com/veesix-networks/osvbng/commit/b9146243a05774d1db7e76e7767ca62d5b14972b))
* **dhcpv6:** use SRG virtual MAC or access interface MAC for proxy DUID ([#176](https://github.com/veesix-networks/osvbng/issues/176)) ([785851f](https://github.com/veesix-networks/osvbng/commit/785851f9ee9553c9042fcc7eb452a6864803e034))
* **ha:** promote WAITING SRGs to ACTIVE_SOLO when peer is unreachable ([#171](https://github.com/veesix-networks/osvbng/issues/171)) ([06e1f15](https://github.com/veesix-networks/osvbng/commit/06e1f15b258e436ea00c8cc0e9cad44f3cb91bc8))

## [0.4.0](https://github.com/veesix-networks/osvbng/compare/v0.3.1...v0.4.0) (2026-03-03)


### Features

* **ha:** add HA foundation with SRG state machine, gRPC peer, and component integration ([#137](https://github.com/veesix-networks/osvbng/issues/137)) ([2df141b](https://github.com/veesix-networks/osvbng/commit/2df141b1d6aeae3a6744855a91dc0baab46750ed))
* **ha:** add interface tracking, SRG counters handler, and split-brain resolution ([#141](https://github.com/veesix-networks/osvbng/issues/141)) ([d6f5c5e](https://github.com/veesix-networks/osvbng/commit/d6f5c5ec6610a169961ba5aa52aed2bf64d93260))
* **ha:** add pool-targeted sync and full bulk sync from live sessions ([#165](https://github.com/veesix-networks/osvbng/issues/165)) ([573e145](https://github.com/veesix-networks/osvbng/commit/573e145e3c48be938ad897bea2ba44a38680c441))
* **ha:** add session sync for HA standby replication ([#164](https://github.com/veesix-networks/osvbng/issues/164)) ([0b2bf44](https://github.com/veesix-networks/osvbng/commit/0b2bf447fcca42849a51ccf50661a34932d8a566))
* **ha:** add SRG BGP route advertisement and withdrawal on failover ([#142](https://github.com/veesix-networks/osvbng/issues/142)) ([1e3613f](https://github.com/veesix-networks/osvbng/commit/1e3613faec9d5ea9483a03b7c1acb09d3d801cfd))
* **ha:** add SRG dataplane abstraction with VPP implementation and no-op fallback ([#140](https://github.com/veesix-networks/osvbng/issues/140)) ([89888e5](https://github.com/veesix-networks/osvbng/commit/89888e57e600bc5ac716ff5a44cb84432779f0d7))


### Bug Fixes

* **ha:** standby does not auto-promote on peer loss ([#163](https://github.com/veesix-networks/osvbng/issues/163)) ([0693571](https://github.com/veesix-networks/osvbng/commit/0693571e0f83cea5d3c54f3cabcfc969f3d03d5b))
* **ipv6:** use separate IPv6 profile name in allocator and add PPPoE IANA allocation ([#159](https://github.com/veesix-networks/osvbng/issues/159)) ([34a6dca](https://github.com/veesix-networks/osvbng/commit/34a6dcae44cd3d811135b22b66a131ba82190c7f))
* poll for bngblaster exit instead of fixed sleep ([#157](https://github.com/veesix-networks/osvbng/issues/157)) ([76d613d](https://github.com/veesix-networks/osvbng/commit/76d613dd88c501d54ccdbcc0fac49a85da16c71d))
* poll for IPv6 readiness in session tests and enable IP6CP for PPPoE ([#158](https://github.com/veesix-networks/osvbng/issues/158)) ([9bd142d](https://github.com/veesix-networks/osvbng/commit/9bd142d45dfbc9d121cf5e9344a3b7fe88860925))

## [0.3.1](https://github.com/veesix-networks/osvbng/compare/v0.3.0...v0.3.1) (2026-02-23)


### Bug Fixes

* **docker:** create dataplane netns in published image entrypoint ([#135](https://github.com/veesix-networks/osvbng/issues/135)) ([81f8530](https://github.com/veesix-networks/osvbng/commit/81f8530199d1e21bcbc496619ffc8f918c700def))

## [0.3.0](https://github.com/veesix-networks/osvbng/compare/v0.2.0...v0.3.0) (2026-02-22)


### Features

* **watchdog:** add VPP health monitoring and dataplane recovery ([#128](https://github.com/veesix-networks/osvbng/issues/128)) ([2bd4648](https://github.com/veesix-networks/osvbng/commit/2bd4648c47bd3daeefb06333b1887d475aaddb0d))


### Bug Fixes

* **ipoe:** always call ip_table_bind for new IPoE session interfaces ([#134](https://github.com/veesix-networks/osvbng/issues/134)) ([ea47a27](https://github.com/veesix-networks/osvbng/commit/ea47a276b3fea19ec5d0e3f3b0b09188d778f6ce))
* **ipoe:** fix VPP crash on session re-creation and release resource leaks ([#133](https://github.com/veesix-networks/osvbng/issues/133)) ([339665f](https://github.com/veesix-networks/osvbng/commit/339665f69b398e0e1b76c2a263723e938e93a240))

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
