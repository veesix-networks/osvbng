# tests/41-upgrade-tier-a-v2

End-to-end QEMU-driven validation of the Tier A v2 upgrade pipeline.
Each scenario boots a fresh CoW overlay of the packer-built osvbng
qcow2, hot-swaps v2 binaries onto the v0.13.0 install (using the
test cosign keypair generated per-run), builds a signed tarball
locally, scp's it in, and drives `osvbngcli upgrade` from the host
via SSH.

Why not the existing clab/docker pattern: the upgrade flow assumes
systemd manages `osvbngd`, with file swaps landing on
`/usr/local/bin/osvbngd` and `systemctl restart` cycling the daemon.
Docker containers run `osvbngd` directly via the entrypoint — no
systemd, no drop-ins, no service supervision — so a docker-based
test is structurally unable to exercise the real code paths.

## Layout

```
tests/41-upgrade-tier-a-v2/
├── README.md
├── harness/
│   └── lib.sh          # sourced by every scenario; owns VM lifecycle,
│                       # keygen, cloud-init seed, bootstrap, tarball push
└── scenarios/
    ├── 01-basic-v2-apply.sh   # happy-path apply + journal + rollback dir
    └── …                      # add new scenarios per upgrade primitive
```

## Prerequisites

- `qemu-system-x86_64` and `xorrisofs` (Debian: `qemu-system-x86`,
  `xorriso`).
- `cosign` on `PATH` (the build-test-tarball helper needs it).
- KVM available (`/dev/kvm` readable; the harness boots with
  `-enable-kvm`).
- A base qcow2 at `/var/lib/libvirt/images/osvbng.qcow2`. The image
  shipped on the GitHub release works; an internal packer build also
  works. Override with `OSVBNG_BASE_IMG=/path/to/image.qcow2`.

## Running

```bash
# Single scenario
./tests/41-upgrade-tier-a-v2/scenarios/01-basic-v2-apply.sh

# Preserve the per-run tmpdir on success (forensics)
OSVBNG_KEEP_STATE=1 ./tests/41-upgrade-tier-a-v2/scenarios/01-basic-v2-apply.sh
```

Each scenario:

1. Generates a fresh SSH keypair + cosign keypair under a per-run
   tmpdir.
2. Boots a CoW overlay of the base image.
3. Hot-swaps v2 binaries + replaces the trust-anchor cosign.pub +
   clears v0.13.0 upgrade state.
4. Drives the test via SSH and asserts outcomes against the on-VM
   journal / current-manifest / rollback dirs.
5. Shuts down qemu and removes the tmpdir on success; preserves on
   failure (state visible at the path the failure message prints).

## What gets tested

| Scenario | Coverage |
|---|---|
| 01-basic-v2-apply | Manifest schema v2 parsed + applied; journal completes; current-manifest updated; rollback snapshot recorded; binary swap took effect. |
| (next) 02-rollback | After a successful apply, `osvbngcli upgrade rollback` restores the prior version. |
| (next) 03-partial-apply-guard | Mid-apply crash leaves a non-completed journal; subsequent apply refuses without `--force-retry`. |
| (next) 04-stepwise | Tarball declaring `previous_version` refuses unless on-disk current matches; prev-manifest sha mismatch refuses. |
| (next) 05-first-boot | `--first-boot` apply against an osvbngd-absent image; journal terminal phase == `first_boot_completed`. |

## Known blockers

The harness boots cleanly and SSH/scp/lifecycle helpers all work, but
`01-basic-v2-apply` does not currently reach `osvbngd state=ready`
after bootstrap. Two upstream issues that need to land before scenarios
can pass end-to-end:

1. **`osvbng.service` is missing `RuntimeDirectoryPreserve=yes`.** The
   shipped systemd unit declares `RuntimeDirectory=osvbng`, so every
   `systemctl stop osvbng.service` (including the upgrade flow's
   stop-swap-start) nukes `/run/osvbng/` — taking vpp's
   `dataplane_api.sock` with it. On next start, vpp.service is still
   active but its socket file is gone and osvbngd refuses to come up.
   The harness installs a transient drop-in via
   `bootstrap_v2()`, but the production unit also needs the fix so an
   in-place upgrade against a real deployment doesn't break dataplane
   continuity. **File-blocking-issue candidate.**
2. **osvbngd auto-binds `eth0` from the system network namespace** into
   VPP at startup, regardless of whether the YAML config declares any
   interfaces. The shipped packer image is configured for a specific
   DPDK NIC topology; a generic test VM whose only NIC is the cloud-init
   management interface fails to come ready because VPP refuses the
   bind (`VPPApiError: Invalid interface (-71)`). Either osvbngd needs
   a "no-dataplane" startup mode, or this harness needs a purpose-built
   test image whose VPP runs without binding any real interface.
   **Out-of-scope for this PR; needs design discussion.**

Both findings came directly from building this harness — the upgrade
flow's correctness on a real systemd-managed box was previously
asserted only by unit tests with a fake `Cmd` stub, which doesn't
exercise the systemd RuntimeDirectory cleanup or VPP interface bind
paths. Worth filing as their own issues; the harness can stay in the
repo as the canonical place to add scenarios once the unblockers land.

## Knobs

| Env var | Default | Effect |
|---|---|---|
| `OSVBNG_BASE_IMG` | `/var/lib/libvirt/images/osvbng.qcow2` | Base image. |
| `OSVBNG_VM_RAM` | `2048` | VM RAM in MiB. |
| `OSVBNG_VM_CPUS` | `2` | VM vCPUs. |
| `OSVBNG_SSH_USER` | `root` | SSH user (cloud-init grants root by default). |
| `OSVBNG_SSH_TIMEOUT` | `180` | Seconds to wait for sshd post-boot. |
| `OSVBNG_KEEP_STATE` | unset | When set (any value), the tmpdir is preserved on success. |

## CI integration

This suite currently runs locally only. It needs a runner with KVM
access; the existing self-hosted runners (`ci/self-hosted-runner`)
host docker containers and don't expose `/dev/kvm`. A follow-up issue
should provision a KVM-capable runner (or wire libvirt into an
existing one) and add a workflow that gates merge on these scenarios.
