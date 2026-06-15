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
# Per docs/operations/upgrade.md the `upgrade` sub-actions are osvbngcli
# builtins, not CLI subcommands. Pipe the command into the REPL.
# Run with `set +e` so we can collect diagnostics on a non-zero exit
# instead of failing immediately; the runner's own health gate may be
# racing systemd state and we want a longer manual settle before
# deciding the upgrade failed.
set +e
vm_ssh "echo 'upgrade apply --force-retry $REMOTE_TARBALL' | osvbngcli"
APPLY_RC=$?
set -e
log "upgrade apply returned rc=$APPLY_RC"

dump_service_status "immediately after upgrade apply returned"

log "Settling 30s for systemd / vpp / osvbngd to converge"
sleep 30
dump_service_status "after 30s settle"

log "Waiting up to 120s for osvbng state=ready"
if wait_for_osvbng_ready 120; then
    ok "osvbng reached state=ready after upgrade"
else
    dump_service_status "osvbng never reached ready after upgrade"
    vm_ssh 'journalctl -u osvbng.service --no-pager -n 60 || true' >&2 || true
    fail "osvbng state never reached ready after upgrade apply"
fi

JOURNAL_JSON="$(vm_ssh 'cat /var/opt/osvbng/upgrade-state.json' 2>/dev/null || true)"
[[ -n "$JOURNAL_JSON" ]] || fail "upgrade-state.json missing"
log "upgrade-state.json:"
log "$JOURNAL_JSON"
grep -qE '"phase"[[:space:]]*:[[:space:]]*"completed"' <<<"$JOURNAL_JSON" || fail "journal phase != completed"
ok "journal phase == completed"

CURRENT_VER="$(vm_ssh "awk '/^osvbng_version:/ {print \$2}' /var/opt/osvbng/current-manifest.yaml" 2>/dev/null)"
[[ "$CURRENT_VER" == "$TARGET_VERSION" ]] || fail "current-manifest osvbng_version=$CURRENT_VER want $TARGET_VERSION"
ok "current-manifest == $TARGET_VERSION"

SNAP_VER="$(vm_ssh 'ls /var/opt/osvbng/rollback/ 2>/dev/null | head -1' || true)"
[[ -n "$SNAP_VER" ]] || fail "no rollback snapshot under /var/opt/osvbng/rollback/"
ok "rollback snapshot $SNAP_VER"

ON_DISK_VER="$(vm_ssh "/usr/local/bin/osvbngd --version 2>/dev/null | head -1" || true)"
case "$ON_DISK_VER" in
    "$TARGET_VERSION"*|*" $TARGET_VERSION"*) ok "osvbngd --version contains $TARGET_VERSION" ;;
    *) fail "osvbngd --version=$ON_DISK_VER want $TARGET_VERSION" ;;
esac

ok "01-basic-v2-apply"
