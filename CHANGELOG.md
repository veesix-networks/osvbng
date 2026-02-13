# Changelog

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
