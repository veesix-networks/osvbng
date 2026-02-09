# Commit Messages

This project uses [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) to automate changelog generation and semantic versioning via [release-please](https://github.com/googleapis/release-please).

## Format

```
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

### Types

| Type | Purpose | Version Bump | Example |
|------|---------|-------------|---------|
| `feat` | New feature | Minor | `feat: add IS-IS, OSPFv2 and OSPFv3 integration` |
| `fix` | Bug fix | Patch | `fix: resolve IPoE session crash on nil reference during teardown` |
| `docs` | Documentation only | None | `docs: add subscriber group configuration guide` |
| `refactor` | Code restructuring | None | `refactor(logging): move component logging to generic sub-logging` |
| `perf` | Performance improvement | None | `perf: replace socket-based punting with shared memory` |
| `test` | Adding or updating tests | None | `test: add handler coverage for show system` |
| `ci` | CI/CD changes | None | `ci: add issue templates and golangci-lint` |
| `chore` | Maintenance / dependencies | None | `chore: update Go to 1.24` |
| `build` | Build system changes | None | `build: update protobuf generation flags` |

!!! note
    Only `feat` and `fix` trigger a release. Other types are included in the changelog but do not bump the version on their own.

### Scopes

Scopes are optional and describe the area of the codebase affected. Use lowercase.

```
feat(dhcpv6): add basic IA_NA and IA_PD support
feat(plugin): add HTTP authentication provider
fix(ipoe): prevent session crash on nil reference during teardown
feat(api): add auto-generated OpenAPI documentation
feat(routing): add IS-IS and OSPF integration
refactor(logging): move component logging to generic sub-logging
```

Common scopes: `dhcp`, `dhcpv6`, `pppoe`, `ppp`, `l2tp`, `ipoe`, `aaa`, `southbound`, `dataplane`, `routing`, `api`, `plugin`, `monitoring`.

### Description

- Use the imperative mood ("add support" not "added support")
- Do not capitalise the first letter
- No period at the end

## Breaking Changes

Breaking changes trigger a **major** version bump. Indicate them in one of two ways:

**With `!` after the type/scope:**

```
feat!: replace memif with shared memory and standardize BNG core config

Memif is no longer used. The dataplane plugin now handles both
ingress and egress for all protocols using a shared memory
implementation.
```

**With a `BREAKING CHANGE` footer:**

```
feat(dataplane): add reconciliation and abstracted configurations

Config handlers now follow idempotency. Anything that touches the
dataplane must first check and then reconcile to the intended
configuration.

BREAKING CHANGE: bootstrap configuration has been replaced by
abstracted config handlers. Existing startup configurations will
need to be regenerated.
```

## Pull Requests and Squash Merging

PRs are squash-merged into `main`, so the **PR title** becomes the commit message. This means:

!!! important
    Your PR title must follow Conventional Commits format.

- The PR title is what appears in the changelog
- Individual commit messages within the PR branch don't need to follow the convention (though it's good practice)
- Keep the PR title concise and meaningful - it's what users will read in the changelog

**Good PR titles:**

- `feat(routing): add IS-IS, OSPFv2 and OSPFv3 integration`
- `feat(plugin): add HTTP authentication provider`
- `fix(ipoe): prevent session crash on nil reference during teardown`
- `feat(monitoring): restructure metric registration for prometheus exporters`

**Bad PR titles:**

- `Basic IGP integration` (no type)
- `Egress backoff + CPPM cleanup` (no type, combines unrelated changes)
- `Fix stuff` (no type, vague)
- `feat: Add HTTP AuthProvider Plugin` (capitalised description)

### Multi-area changes

A fix or feature may touch multiple areas of the codebase. The PR title should describe the **overall outcome**, not list every subsystem affected. Use the PR body for the details.

For example, a dual-stack teardown crash that requires changes across DHCPv6, IPoE, and the dataplane plugin:

**PR title:**
```
fix(ipoe): resolve subscriber session crash during dual-stack teardown
```

**PR body:**
```
DHCPv6 binding was racing with IPoE teardown, causing a nil reference
when the dataplane plugin tried to clean up the session.

- Fixed DHCPv6 lease state cleanup ordering
- Added nil guard in IPoE session teardown path
- Ensured dataplane plugin checks session validity before deletion
```

The scope should reflect the **primary area** of the change. The title captures what was fixed and why (the changelog entry), while the body explains how.

!!! warning "Don't combine unrelated changes"
    If a PR genuinely contains unrelated changes (e.g., a bug fix **and** a new feature), split it into separate PRs. Each gets its own changelog entry and appropriate version bump.

## How This Drives Releases

[release-please](https://github.com/googleapis/release-please) monitors `main` and automatically:

1. Opens a **Release PR** that accumulates changes, updates the changelog, and bumps the version
2. When the Release PR is merged, it creates a **Git tag** and **GitHub Release**
3. The tag triggers the Docker image build and publish

You don't need to manually tag releases or maintain a changelog - just write good commit messages.
