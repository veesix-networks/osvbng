#!/bin/bash
# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Boot a CoW copy of the base qcow2, hot-swap v2 binaries onto the
# v0.13.0 install, build + sign a v2 tarball locally, scp it onto the
# VM, run `osvbngcli upgrade apply --force-retry`, and assert:
#
#   - exit 0
#   - upgrade-state.json terminal phase == "completed"
#   - current-manifest.yaml osvbng_version == target
#   - rollback snapshot dir is present
#
# --force-retry is required because the bootstrap step deletes the v1
# journal but the new v2 osvbngd may still see a stale state on first
# read. Subsequent scenarios that exercise the partial-apply guard turn
# this off.

set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/../harness/lib.sh"

trap trap_cleanup EXIT

setup_keys
build_cloud_init_seed
boot_vm
wait_for_ssh
bootstrap_v2

TARGET_VERSION="0.14.1"
build_and_push_tarball "$TARGET_VERSION" REMOTE_TARBALL

log "Driving osvbngcli upgrade apply ($REMOTE_TARBALL)"
vm_ssh "osvbngcli upgrade apply --force-retry $REMOTE_TARBALL" || fail "upgrade apply returned non-zero"

log "Asserting terminal journal phase == completed"
JOURNAL_JSON="$(vm_ssh 'cat /var/opt/osvbng/upgrade-state.json' 2>/dev/null || true)"
[[ -n "$JOURNAL_JSON" ]] || fail "upgrade-state.json missing post-apply"
if ! grep -q '"phase":"completed"' <<<"$JOURNAL_JSON"; then
    log "Journal contents: $JOURNAL_JSON"
    fail "journal terminal phase is not 'completed'"
fi
ok "journal terminal phase == completed"

log "Asserting current-manifest reports v$TARGET_VERSION"
CURRENT_VER="$(vm_ssh "awk '/^osvbng_version:/ {print \$2}' /var/opt/osvbng/current-manifest.yaml" 2>/dev/null)"
[[ "$CURRENT_VER" == "$TARGET_VERSION" ]] || fail "current-manifest osvbng_version is '$CURRENT_VER', want '$TARGET_VERSION'"
ok "current-manifest reflects v$TARGET_VERSION"

log "Asserting rollback snapshot present"
SNAP_VER="$(vm_ssh 'ls /var/opt/osvbng/rollback/ 2>/dev/null | head -1' || true)"
[[ -n "$SNAP_VER" ]] || fail "no rollback snapshot dir under /var/opt/osvbng/rollback/"
ok "rollback snapshot present at /var/opt/osvbng/rollback/$SNAP_VER"

log "Asserting on-disk osvbngd contains v$TARGET_VERSION (binary swap took effect)"
ON_DISK_VER="$(vm_ssh "/usr/local/bin/osvbngd --version 2>/dev/null | awk '/[Vv]ersion/ {print \$NF; exit}'" || true)"
case "$ON_DISK_VER" in
    *"$TARGET_VERSION"*) ok "osvbngd --version contains $TARGET_VERSION ($ON_DISK_VER)" ;;
    *) fail "osvbngd --version returned '$ON_DISK_VER', expected to contain $TARGET_VERSION" ;;
esac

ok "01-basic-v2-apply: PASS"
