#!/bin/bash
# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

# Apply v0.14.1 over v0.14.0-test, then invoke `upgrade rollback` and
# assert: journal terminal phase == rolled_back, daemon comes back
# ready on the prior version, on-disk osvbngd --version reports the
# prior version. The runner intentionally does NOT rewrite
# current-manifest.yaml on rollback (it keeps pointing at the
# forward-applied version), so the manifest is not part of the
# post-rollback contract — only the live binary + journal phase are.

set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/../harness/lib.sh"

trap trap_cleanup EXIT

setup_keys
build_cloud_init_seed
boot_vm
wait_for_ssh
bootstrap_v2

FROM_VERSION="0.14.0-test"
TO_VERSION="0.14.1"
build_and_push_tarball "$TO_VERSION" REMOTE_TARBALL

log "upgrade apply $REMOTE_TARBALL"
set +e
vm_ssh "echo 'upgrade apply --force-retry $REMOTE_TARBALL' | osvbngcli"
APPLY_RC=$?
set -e
log "upgrade apply returned rc=$APPLY_RC"
dump_service_status "after upgrade apply"

log "Waiting up to 120s for osvbng state=ready (post-apply)"
wait_for_osvbng_ready 120 || {
    dump_service_status "osvbng not ready after apply"
    vm_ssh 'journalctl -u osvbng.service --no-pager -n 40 || true' >&2 || true
    fail "osvbng never reached ready after apply (rollback test prerequisite)"
}
ok "osvbng ready at $TO_VERSION"

POST_APPLY_VER="$(vm_ssh "/usr/local/bin/osvbngd --version 2>/dev/null | head -1" || true)"
case "$POST_APPLY_VER" in
    "$TO_VERSION"*|*" $TO_VERSION"*) ok "post-apply osvbngd reports $TO_VERSION" ;;
    *) fail "post-apply osvbngd --version=$POST_APPLY_VER want $TO_VERSION" ;;
esac

log "upgrade rollback"
set +e
vm_ssh "echo 'upgrade rollback' | osvbngcli"
ROLLBACK_RC=$?
set -e
log "upgrade rollback returned rc=$ROLLBACK_RC"
dump_service_status "after upgrade rollback"

log "Waiting up to 120s for osvbng state=ready (post-rollback)"
wait_for_osvbng_ready 120 || {
    dump_service_status "osvbng not ready after rollback"
    vm_ssh 'journalctl -u osvbng.service --no-pager -n 40 || true' >&2 || true
    fail "osvbng never reached ready after rollback"
}
ok "osvbng ready after rollback"

JOURNAL_JSON="$(vm_ssh 'cat /var/opt/osvbng/upgrade-state.json' 2>/dev/null || true)"
[[ -n "$JOURNAL_JSON" ]] || fail "upgrade-state.json missing after rollback"
log "upgrade-state.json:"
log "$JOURNAL_JSON"
grep -qE '"phase"[[:space:]]*:[[:space:]]*"rolled_back"' <<<"$JOURNAL_JSON" || fail "journal phase != rolled_back"
ok "journal phase == rolled_back"

POST_ROLLBACK_VER="$(vm_ssh "/usr/local/bin/osvbngd --version 2>/dev/null | head -1" || true)"
case "$POST_ROLLBACK_VER" in
    "$FROM_VERSION"*|*" $FROM_VERSION"*) ok "post-rollback osvbngd reports $FROM_VERSION" ;;
    *) fail "post-rollback osvbngd --version=$POST_ROLLBACK_VER want $FROM_VERSION" ;;
esac

vm_ssh "grep -q '\"version\":\"$FROM_VERSION\"' /run/osvbng/state" 2>/dev/null \
    || fail "/run/osvbng/state does not report version=$FROM_VERSION (contents: $(vm_ssh 'cat /run/osvbng/state' 2>/dev/null))"
ok "/run/osvbng/state version == $FROM_VERSION"

ok "02-rollback"
