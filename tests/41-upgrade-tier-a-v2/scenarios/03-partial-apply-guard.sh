#!/bin/bash
# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

# Plant a fake non-terminal upgrade-state.json (simulating a crashed
# prior apply), then assert: a plain `upgrade apply <tarball>` refuses
# with a clear "previous upgrade is in non-completed state" error,
# while `upgrade apply --force-retry <tarball>` proceeds and completes.

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

log "Planting fake non-terminal journal (phase=aborted_pre_swap)"
vm_ssh_in <<'PLANT'
set -e
mkdir -p /var/opt/osvbng
cat > /var/opt/osvbng/upgrade-state.json <<'EOF'
{
  "version": 1,
  "from": "0.14.0-test",
  "to": "0.14.0-fake-prior-attempt",
  "tarball": "/tmp/nonexistent.tar.gz",
  "started_at": "2026-01-01T00:00:00Z",
  "updated_at": "2026-01-01T00:00:05Z",
  "phase": "aborted_pre_swap",
  "completed_artifacts": []
}
EOF
PLANT

log "apply WITHOUT --force-retry should refuse"
APPLY_OUT="$(vm_ssh "echo 'upgrade apply $REMOTE_TARBALL' | osvbngcli" 2>&1 || true)"
log "apply output:"
log "$APPLY_OUT"
grep -q "non-completed state" <<<"$APPLY_OUT" \
    || fail "expected 'non-completed state' refusal, got: $APPLY_OUT"
grep -q "force-retry" <<<"$APPLY_OUT" \
    || fail "expected refusal to mention --force-retry, got: $APPLY_OUT"
ok "apply without --force-retry refused as expected"

# State file should still be the planted one — refusal must not touch it.
PHASE_AFTER_REFUSAL="$(vm_ssh "cat /var/opt/osvbng/upgrade-state.json" 2>/dev/null | grep -oE '"phase"[[:space:]]*:[[:space:]]*"[^"]+"' || true)"
case "$PHASE_AFTER_REFUSAL" in
    *aborted_pre_swap*) ok "planted journal unmodified by refusal" ;;
    *) fail "expected planted journal preserved, got: $PHASE_AFTER_REFUSAL" ;;
esac

log "apply WITH --force-retry should succeed"
set +e
vm_ssh "echo 'upgrade apply --force-retry $REMOTE_TARBALL' | osvbngcli"
APPLY_RC=$?
set -e
log "apply --force-retry returned rc=$APPLY_RC"
dump_service_status "after apply --force-retry"

log "Waiting up to 120s for osvbng state=ready"
wait_for_osvbng_ready 120 || {
    dump_service_status "osvbng not ready after force-retry apply"
    vm_ssh 'journalctl -u osvbng.service --no-pager -n 40 || true' >&2 || true
    fail "osvbng never reached ready after --force-retry apply"
}
ok "osvbng ready after --force-retry apply"

JOURNAL_JSON="$(vm_ssh 'cat /var/opt/osvbng/upgrade-state.json' 2>/dev/null || true)"
[[ -n "$JOURNAL_JSON" ]] || fail "upgrade-state.json missing"
log "upgrade-state.json:"
log "$JOURNAL_JSON"
grep -qE '"phase"[[:space:]]*:[[:space:]]*"completed"' <<<"$JOURNAL_JSON" \
    || fail "journal phase != completed after force-retry"
ok "journal phase == completed"

vm_ssh "grep -q '\"version\":\"$TARGET_VERSION\"' /run/osvbng/state" 2>/dev/null \
    || fail "/run/osvbng/state version != $TARGET_VERSION (contents: $(vm_ssh 'cat /run/osvbng/state' 2>/dev/null))"
ok "/run/osvbng/state version == $TARGET_VERSION"

ok "03-partial-apply-guard"
