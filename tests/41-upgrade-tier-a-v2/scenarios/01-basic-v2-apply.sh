#!/bin/bash
# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

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

log "upgrade apply $REMOTE_TARBALL"
vm_ssh "osvbngcli upgrade apply --force-retry $REMOTE_TARBALL" || fail "upgrade apply non-zero"

JOURNAL_JSON="$(vm_ssh 'cat /var/opt/osvbng/upgrade-state.json' 2>/dev/null || true)"
[[ -n "$JOURNAL_JSON" ]] || fail "upgrade-state.json missing"
grep -q '"phase":"completed"' <<<"$JOURNAL_JSON" || { log "$JOURNAL_JSON"; fail "journal phase != completed"; }
ok "journal phase == completed"

CURRENT_VER="$(vm_ssh "awk '/^osvbng_version:/ {print \$2}' /var/opt/osvbng/current-manifest.yaml" 2>/dev/null)"
[[ "$CURRENT_VER" == "$TARGET_VERSION" ]] || fail "current-manifest osvbng_version=$CURRENT_VER want $TARGET_VERSION"
ok "current-manifest == $TARGET_VERSION"

SNAP_VER="$(vm_ssh 'ls /var/opt/osvbng/rollback/ 2>/dev/null | head -1' || true)"
[[ -n "$SNAP_VER" ]] || fail "no rollback snapshot under /var/opt/osvbng/rollback/"
ok "rollback snapshot $SNAP_VER"

ON_DISK_VER="$(vm_ssh "/usr/local/bin/osvbngd --version 2>/dev/null | awk '/[Vv]ersion/ {print \$NF; exit}'" || true)"
case "$ON_DISK_VER" in
    *"$TARGET_VERSION"*) ok "osvbngd --version contains $TARGET_VERSION" ;;
    *) fail "osvbngd --version=$ON_DISK_VER want $TARGET_VERSION" ;;
esac

ok "01-basic-v2-apply"
