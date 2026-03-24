# Getting Started

!!! abstract "Community Guidelines"
    - Be respectful
    - Don't be a dick
    - Be a part of the solution, not the problem

## Prerequisites

- Go 1.24+
- Protocol Buffers compiler (`protobuf-compiler`)
- [golangci-lint](https://golangci-lint.run/docs/welcome/install/local/)
- Docker & Docker Compose (for local testing)

## Clone and Build

```bash
git clone https://github.com/veesix-networks/osvbng.git
cd osvbng
make build
```

This produces `bin/osvbngd` and `bin/osvbngcli`.

## Running Locally

```bash
make build
cd docker/dev
./dev.sh start
```

This builds the Docker image, creates the network topology, and starts osvbng.

## Development

### Branch Naming

Use prefixed branch names:

| Prefix | Purpose |
|--------|---------|
| `feat/` | New features |
| `fix/` | Bug fixes |
| `perf/` | Performance improvements |
| `ci/` | CI/CD changes |
| `refactor/` | Code restructuring |
| `docs/` | Documentation |
| `test/` | Test changes |

### Code Style

!!! warning "Before submitting"
    Always run linting before opening a PR.

```bash
make lint   # check for issues
make fmt    # auto-fix formatting
```

### Running Tests

```bash
make test          # run unit tests
make test-report   # run tests with JUnit XML report in build/reports/
```

**Unit tests** (`make test`) validate individual packages in isolation - parsers, allocators, config handling, protocol logic. They run fast, require no infrastructure, and are the first gate in CI.

**Integration tests** (`tests/`) use containerlab and robot framework to spin up a full osvbng instance with BNGBlaster subscribers, verifying end-to-end functionality across all supported features. They require Docker and take longer to run.

CI runs unit tests first. Integration tests will not run until unit tests pass. Both must pass before a PR can be merged.

!!! note "Future: Hardware and QEMU testing"
    We plan to build physical and QEMU-based test infrastructure for throughput and subscriber scaling validation on minor and major releases.

### Profiling

For performance work, set `OSVBNG_PROFILE=1` in the container environment to enable pprof endpoints and runtime profiling (block rate + mutex fraction):

```bash
# CPU profile (capture during load test)
curl -o cpu.prof http://<bng-ip>:8080/debug/pprof/profile?seconds=30

# Block profile (where goroutines are blocked - mutexes, channels, syscalls)
curl -o block.prof http://<bng-ip>:8080/debug/pprof/block

# Mutex contention (which mutexes are contended and for how long)
curl -o mutex.prof http://<bng-ip>:8080/debug/pprof/mutex

# Goroutine dump (what all goroutines are doing right now)
curl -o goroutine.txt "http://<bng-ip>:8080/debug/pprof/goroutine?debug=2"

# Execution trace (goroutine scheduling, blocking, syscalls over time)
curl -o trace.out "http://<bng-ip>:8080/debug/pprof/trace?seconds=5"
```

Analyze with:

```bash
go tool pprof -top cpu.prof          # CPU hot functions
go tool pprof -top -cum block.prof   # where goroutines spend time blocked
go tool pprof -top -cum mutex.prof   # mutex contention ranking
go tool trace trace.out              # visual goroutine timeline
```

## New Features

!!! note "Before you start coding"
    Open an [issue](https://github.com/veesix-networks/osvbng/issues) first to discuss your idea with the core developers. This helps ensure your contribution aligns with the project direction and avoids wasted effort.

## Submitting Changes

1. Fork the repo
2. Create a branch (`git checkout -b <type>/my-change`)
3. Make your changes
4. Run `make lint` and `make test`
5. Commit using [Conventional Commits](commit-messages.md) format and push
6. Open a PR against `main` - the PR title must follow Conventional Commits format (PRs are squash-merged)

!!! important "Documentation"
    Any contribution that adds, modifies, or removes behaviour must include corresponding documentation updates.

## Questions?

Join the [Discord](https://dsc.gg/osvbng), open a [discussion](https://github.com/veesix-networks/osvbng/discussions), or file an [issue](https://github.com/veesix-networks/osvbng/issues).
