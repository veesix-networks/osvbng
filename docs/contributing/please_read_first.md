# Contributing

## Getting Started

### Prerequisites

- Go 1.24+
- [golangci-lint](https://golangci-lint.run/docs/welcome/install/local/)
- Docker & Docker Compose (for local testing)

### Clone and Build

```bash
git clone https://github.com/veesix-networks/osvbng.git
cd osvbng
make build
```

This produces `bin/osvbngd` and `bin/osvbngcli`.

### Running Locally

The dev environment uses Docker Compose with VPP and bngblaster:

```bash
make build
cd docker/dev
docker-compose up
```

## Development

### Code Style

Before submitting, run:

```bash
make lint   # check for issues
make fmt    # auto-fix formatting
```

### Running Tests

```bash
make test
```

## Submitting Changes

1. Fork the repo
2. Create a branch (`git checkout -b feature/my-feature`)
3. Make your changes
4. Run `make lint` and `make test`
5. Commit and push
6. Open a PR against `main`

## Questions?

Open a [discussion](https://github.com/veesix-networks/osvbng/discussions) or [issue](https://github.com/veesix-networks/osvbng/issues).
