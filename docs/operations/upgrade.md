# Upgrades

This page covers the in-place upgrade workflow for osvbng deployments
on QEMU/baremetal: `osvbngcli upgrade plan / apply / rollback / status`.

Docker users do not use this flow — Docker upgrades go through
`docker pull veesixnetworks/osvbng:vX.Y.Z` and a container restart.
The `osvbngcli upgrade` builtin is for systemd-supervised deployments
where osvbng owns the box.

## Operator UX

All four sub-actions are issued from inside `osvbngcli`. Don't launch
`osvbngcli`, exit, then run a shell command — the upgrade flow keeps
state in the running `osvbngcli` process and the daemon restarts
gracefully under it.

```
$ osvbngcli
osvbng> upgrade status
osvbng> upgrade plan /tmp/osvbng-v0.13.1.tar.gz
osvbng> upgrade apply /tmp/osvbng-v0.13.1.tar.gz
osvbng> upgrade rollback
```

### `upgrade plan <tarball>`

Read-only dry-run. Extracts the tarball into a sibling-directory of the
tarball itself (so a 2GB tarball on `/tmp` does not silently land on a
different partition), verifies the signature, parses the manifest,
checks for drift against the current installed artifacts, and prints a
summary. Removes the staging directory on exit. **No side effects on
production paths.**

### `upgrade apply <tarball>`

Applies the tarball in-place. The flow is journaled per-step at
`/var/opt/osvbng/upgrade-state.json`, so an interruption at any phase
(operator Ctrl+C, crash, power loss) leaves enough state on disk for
a later `upgrade rollback` to restore from a known partial state.

Stages (14 total):

1. Stage tarball (sibling-dir extraction).
2. Verify signature against `/etc/osvbng/release-keys/cosign.pub`.
3. Parse manifest, run Tier-A scope guard, cross-check artifact hashes.
4. Ensure state directories exist.
5. Drift detection (warn-not-refuse).
6. Snapshot current version to `/var/opt/osvbng/rollback/<from-version>/`.
7. Pre-apply hook (optional, read-only validation).
8. Suspend systemd auto-restart via `/run/systemd/system/osvbng.service.d/upgrade.conf`.
9. `systemctl stop osvbng.service`.
10. Per-artifact atomic swap via `rename(2)`.
11. `systemctl start osvbng.service`.
12. Health poll (systemd `ActiveState` + `/run/osvbng/state`).
13. Commit (write `current-manifest.yaml`, prune snapshots).
14. Post-apply hook (optional, advisory).

A health-check failure auto-triggers rollback; a hook failure aborts
without auto-rollback unless past the snapshot stage.

### `upgrade rollback`

Restores the most recent snapshot. Reads the journal to find the
target version, replays the snapshot's metadata + bytes in reverse
order, restarts the daemon, and health-polls.

Idempotent: a missing journal yields a clean "no rollback available"
message rather than an error.

### `upgrade status`

Reads `/var/opt/osvbng/current-manifest.yaml`,
`/var/opt/osvbng/upgrade-state.json`, and the contents of
`/var/opt/osvbng/rollback/`. Shows current installed version, last
upgrade outcome, in-flight journal phase if any, available rollbacks.
No daemon interaction required — works when osvbngd is stopped.

## Trust model

osvbng release tarballs are signed by the project. The QEMU image
ships with the project's public verification key embedded at
`/etc/osvbng/release-keys/cosign.pub`. When you run `upgrade apply
<tarball>`, the tarball's detached `.sig` sidecar is checked against
that public key before any host-mutating step.

You don't need to manage any keys yourself. The same project key
signs every release. If a signature check fails the apply refuses and
the offending tarball is moved to
`/var/opt/osvbng/quarantine/<sha-prefix>/` for investigation.

### Threat model

The signature check protects the bytes of a release tarball in transit
from the project's release artefact to the operator's box. That is the
guarantee, and it is meaningful — an operator who downloads
`osvbng-vX.Y.Z.tar.gz` from a project release gets exactly the bytes
the project published. Tampering during download, mirror compromise,
or substitution of the GitHub release artefact surfaces as a signature
failure and the apply refuses.

The signature check does **not** protect against:

- **A root-equivalent attacker on the box itself.** Once someone has
  root, they don't need to defeat the upgrade flow — they can replace
  `/usr/local/bin/osvbngd` or `/etc/osvbng/release-keys/cosign.pub`
  directly, or bypass `osvbngcli` entirely. Host hardening (AppArmor,
  read-only filesystems, integrity monitoring) is the right answer
  here; signing isn't.

- **Build-time supply-chain compromise.** A malicious dependency or a
  pre-merge insider change produces a tarball that signs cleanly,
  because the project signs whatever the build pipeline emits.
  Reproducible builds and a published SBOM are the orthogonal
  mitigations.

Signing tells you "this is what the project shipped." It does not
tell you "this is safe to apply." Treat it as one control among
several.

## Recovery from a bricked upgrade

If `upgrade apply` fails its health-check, **auto-rollback fires
immediately** and the previous version is restored. The journal
records `health_failed` then `rolled_back`. `upgrade status` shows
the result.

If both the new daemon AND the rollback target fail health
(`rollback_failed` in the journal), the box is in an unknown state.
Recovery:

1. Serial console always works on QEMU images. `virsh console <vm>`
   or `qm terminal <id>` lets you in as root.
2. From root, inspect `/var/opt/osvbng/upgrade-state.json` to see the
   last journal phase. Inspect `/var/opt/osvbng/rollback/<old>/` for
   the snapshot bytes and metadata.
3. Manual restore: copy snapshot files back into their target paths,
   set `uid:gid:mode` per `/var/opt/osvbng/rollback/<old>/metadata.yaml`,
   then `systemctl start osvbng.service`.

The systemd drop-in at `/run/systemd/system/osvbng.service.d/upgrade.conf`
is automatically cleared on reboot (it lives under `/run`), so a
panic-reboot during the apply window naturally restores `Restart=on-failure`.

## Troubleshooting

### `upgrade apply` refuses the tarball

| Error | Cause | Resolution |
|---|---|---|
| `tarball is unsigned` | The tarball has no `.sig` sidecar | Re-download the tarball; the `.sig` ships alongside it in the GitHub release. Verify both files are present in the same directory. |
| `signature verification failed` | The tarball's signature does not match the project's public key | The tarball is tampered or came from a different source. The offending file is moved to `/var/opt/osvbng/quarantine/<sha-prefix>/` for investigation. Do not attempt to apply again. |
| `tarball declares tier "B"` | The tarball is an apt-bundle upgrade | The apt-bundle upgrade flow is not yet released. Wait for a future osvbng version that supports it. |

### Operator-modified files trigger a drift warning

```
! WARN: operator-modified /usr/share/osvbng/templates/frr.j2 will be
        overwritten by upgrade (manifest expects sha256:abc..., found
        sha256:def...); rollback snapshot will preserve the modified bytes
```

This is informational, not blocking. The apply proceeds. The rollback
snapshot captures your modified bytes so you can recover them after the
upgrade if needed. Hand-edits to managed paths are unsupported — for
persistent local changes, use the operator-facing config schema rather
than editing files in `/usr/share/osvbng/templates/` directly.

### `osvbngcli` session crashes mid-apply

`osvbngcli` keeps its in-memory binary mapped from the old inode even
after the on-disk `osvbngcli` is replaced (Linux open-file semantics),
so the running session should survive the daemon restart. If you do
lose the session (crash, terminal close, etc.) DURING an apply:

1. The journal at `/var/opt/osvbng/upgrade-state.json` records the
   last completed phase.
2. Re-launch `osvbngcli`. The new osvbngcli is whatever was most
   recently swapped to disk.
3. `upgrade status` shows the in-flight phase.
4. `upgrade rollback` cleans up: it reads the journal, finds which
   artefacts had been swapped, restores them in reverse order, and
   restarts the daemon.

### Two-Ctrl+C escape

The first Ctrl+C during an apply cancels the upgrade context — the
flow runs its own cleanup (remove systemd drop-in, restart daemon,
attempt rollback, journal final phase). A SECOND Ctrl+C during the
cleanup window forces `os.Exit(2)` and leaves whatever phase the
first signal got us to recorded in the journal; `upgrade rollback`
can then take over.

## Reference paths on the box

- Trust anchor: `/etc/osvbng/release-keys/cosign.pub`
- State files: `/var/opt/osvbng/{current-manifest.yaml, upgrade-state.json}`
  and `/var/opt/osvbng/rollback/<version>/`
- Runtime state file: `/run/osvbng/state` (daemon writes; upgrade
  health-poll reads).
