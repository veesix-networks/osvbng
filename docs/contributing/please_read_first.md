# Getting Started

!!! abstract "Community Guidelines"
    - Be respectful
    - Don't be a dick
    - Be a part of the solution, not the problem

## Prerequisites

- Go 1.24+
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
docker-compose up
```

## Development

### Code Style

!!! warning "Before submitting"
    Always run linting before opening a PR.

```bash
make lint   # check for issues
make fmt    # auto-fix formatting
```

### Running Tests

```bash
make test
```

## New Features

!!! note "Before you start coding"
    Open an [issue](https://github.com/veesix-networks/osvbng/issues) first to discuss your idea with the core developers. This helps ensure your contribution aligns with the project direction and avoids wasted effort.

## Submitting Changes

1. Fork the repo
2. Create a branch (`git checkout -b my-feature`)
3. Make your changes
4. Run `make lint` and `make test`
5. Commit using [Conventional Commits](commit-messages.md) format and push
6. Open a PR against `main` — the PR title must follow Conventional Commits format (PRs are squash-merged)

!!! important "Documentation"
    Any contribution that adds, modifies, or removes behaviour must include corresponding documentation updates.

## Questions?

Open a [discussion](https://github.com/veesix-networks/osvbng/discussions) or [issue](https://github.com/veesix-networks/osvbng/issues).
