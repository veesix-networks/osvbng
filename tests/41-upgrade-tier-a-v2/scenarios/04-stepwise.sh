#!/bin/bash
# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

# Build a v0.14.1 tarball that declares previous_version=0.13.0 (built
# with --prev), then assert: applying it on a v0.14.0-test box refuses
# with the runner's "stepwise upgrade required" message — the current
# on-disk version doesn't match the tarball's PreviousVersion.

set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/../harness/lib.sh"

trap trap_cleanup EXIT

setup_keys
build_cloud_init_seed
boot_vm
wait_for_ssh
bootstrap_v2

# Build a v0.13.0 tarball to serve as the "previous" the v0.14.1
# tarball will declare. We push it to the VM as a side-effect of the
# helper but only use the host-side copy as input for the next build.
build_and_push_tarball "0.13.0" PREV_TARBALL_REMOTE
PREV_TARBALL_LOCAL="$TEST_DIR/tarballs/osvbng-v0.13.0.tar.gz"
[[ -f "$PREV_TARBALL_LOCAL" ]] || fail "expected prev tarball at $PREV_TARBALL_LOCAL"

# Build v0.14.1 with the v0.13.0 manifest embedded under prev/. The
# resulting v0.14.1 manifest will carry previous_version=0.13.0 plus
# the prev-manifest SHA.
TARGET_VERSION="0.14.1"
build_and_push_tarball "$TARGET_VERSION" REMOTE_TARBALL "$PREV_TARBALL_LOCAL"

# Sanity: confirm the v0.14.1 manifest actually declares the stepwise
# fields — otherwise the test below would pass for the wrong reason.
log "Verifying v$TARGET_VERSION manifest declares previous_version=0.13.0"
MANIFEST="$(tar -xzOf "$TEST_DIR/tarballs/osvbng-v$TARGET_VERSION.tar.gz" ./manifest.yaml)"
grep -q '^previous_version: 0.13.0$' <<<"$MANIFEST" \
    || fail "stepwise tarball missing previous_version: 0.13.0 (got: $MANIFEST)"
grep -q '^previous_manifest_sha256:' <<<"$MANIFEST" \
    || fail "stepwise tarball missing previous_manifest_sha256"
ok "v$TARGET_VERSION manifest declares stepwise prev=0.13.0"

log "apply v$TARGET_VERSION on v0.14.0-test should refuse (version mismatch)"
APPLY_OUT="$(vm_ssh "echo 'upgrade apply --force-retry $REMOTE_TARBALL' | osvbngcli" 2>&1 || true)"
log "apply output:"
log "$APPLY_OUT"
grep -q "stepwise upgrade required" <<<"$APPLY_OUT" \
    || fail "expected 'stepwise upgrade required' refusal, got: $APPLY_OUT"
grep -q "install v0.13.0 first" <<<"$APPLY_OUT" \
    || fail "expected refusal to name the missing predecessor v0.13.0, got: $APPLY_OUT"
grep -q "current is v0.14.0-test" <<<"$APPLY_OUT" \
    || fail "expected refusal to name the on-disk version v0.14.0-test, got: $APPLY_OUT"
ok "stepwise apply refused with the expected error"

# The refusal must not change live state — osvbng should still be on
# the prior version, no swap, no rollback.
vm_ssh "grep -q '\"version\":\"0.14.0-test\"' /run/osvbng/state" 2>/dev/null \
    || fail "/run/osvbng/state changed despite stepwise refusal (contents: $(vm_ssh 'cat /run/osvbng/state' 2>/dev/null))"
ok "/run/osvbng/state still reports 0.14.0-test"

ok "04-stepwise"
