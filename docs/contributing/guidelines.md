# Contribution Guidelines

## Protocol Implementations

When contributing to low-level protocol implementations (e.g., DHCP, PPPoE, RADIUS), you must provide a list of all sources in your PR description.

!!! info "Required Sources"
    Include references to RFCs, IEEE standards, vendor specifications, or other authoritative documentation that informed your implementation.

## Testing Requirements

CI runs build and unit tests on every PR using GitHub shared runners.
Integration tests (containerlab + Robot Framework) run on a self-hosted
runner only when changes are merged to main.

### Running integration tests locally

GitHub shared runners cannot reliably run the integration suite due to
nested QEMU virtualisation and resource constraints. Contributors must
run integration tests locally before requesting a merge.

Prerequisites:

- [containerlab](https://containerlab.dev/install/)
- [Robot Framework](https://robotframework.org/) (`pip install robotframework`)
- Docker

Steps:

1. Run `make test` and fix any unit test failures.
2. Build the local Docker image: `make docker-local`
3. Run the full QA suite: `bash scripts/run-qa-tests.sh -r 1`
4. Include the summary output from `run-qa-tests.sh` in your PR description.

!!! warning "When is a full integration run required?"
    For minor and major version bumps the full QA suite must pass. Patch
    fixes that are fully covered by unit tests do not require a full
    integration run, but the maintainer merging the PR may request one.

When adding new functionality:

- Add unit tests for new packages and functions
- Update existing tests if you change behaviour
- Test with BNGBlaster for session setup changes (`docker/dev/bngblaster/`)

## Performance Guidelines

osvbng processes thousands of subscriber sessions per second. Code in the session setup hot path must be written with performance in mind.

!!! danger "Hot Path Rules"
    - **No mutex contention**: use `sync.Map` or per-object locks instead of global mutexes
    - **No gopacket in DHCP processing**: use the custom binary parsers in `pkg/dhcp4/` and `pkg/dhcp6/`
    - **No blocking I/O under locks**: logging, database writes, and network calls must not hold shared locks
    - **O(1) lookups**: use indexed maps, not linear scans, for anything called per-packet
    - **Minimize allocations**: reuse buffers, avoid `fmt.Sprintf` in hot paths

The session setup path is: VPP punt -> SHM -> dataplane component -> IPoE/PPPoE component -> DHCP provider -> egress SHM -> VPP. Every allocation and lock in this chain affects CPS.

## AI / LLM Usage

We don't prohibit the use of AI tools.

!!! tip "If you use AI"
    - PRs and documentation should reflect human effort and understanding
    - Review and understand AI-generated code before submitting
    - Include the specification/context file you used to prompt the AI in your PR description
    - For protocol implementations, you must still provide RFC/specification sources

This helps reviewers understand your approach and makes the contribution more valuable to the project.

## Code Organisation

| Directory | Purpose |
|-----------|---------|
| `internal/` | Core components (IPoE, PPPoE, ARP, dataplane, subscriber, AAA) |
| `plugins/` | Pluggable providers (DHCP local/relay/proxy, auth local/radius/http, prometheus, northbound API) |
| `pkg/` | Shared libraries (config, logger, allocator, ifmgr, event bus, VPP southbound, HA) |
| `cmd/` | Binary entry points (osvbngd, osvbngcli) |
| `docker/` | Docker and dev environment configs |
| `tests/` | Robot framework integration tests |
| `docs/` | MkDocs documentation |

## Copyright Headers

Every new file must include the SPDX copyright header.

**Go/Proto files:**
```go
// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later
```

**Shell/YAML/Makefile/Dockerfile:**
```bash
# Copyright 2026 Veesix Networks Ltd
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later
```
