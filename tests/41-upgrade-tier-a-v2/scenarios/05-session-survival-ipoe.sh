#!/bin/bash
# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

# Validates a Tier-A v2 upgrade against a BNG with live IPoE
# subscribers. Unlike scenarios 01-04 (single-VM, binary-version
# asserts only), this stands up the 03-ipoe-local containerlab
# topology via the --qemu wrapper, establishes a subscriber session,
# triggers the upgrade mid-flight, and measures the subscriber-visible
# outage. The journal-phase asserts from scenario 01 are kept so a
# pure-control-plane regression still fails this run; the new asserts
# below are about the data plane staying intact (or at least
# converging quickly) across the apply.

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$HERE/../../.." && pwd)"
LAB_NAME="osvbng-ipoe-local"
LAB_DIR="$REPO_ROOT/tests/03-ipoe-local"
TOPO="$LAB_DIR/03-ipoe-local.clab.yml"
BNG="clab-${LAB_NAME}-bng1"
SUBS="clab-${LAB_NAME}-subscribers"
TARGET_VERSION="0.14.1"
EXPECTED_SESSIONS=1
RECOVERY_BUDGET_S=120

log()  { printf '\033[1;34m[%s]\033[0m %s\n' "$(date +%H:%M:%S)" "$*" >&2; }
ok()   { printf '\033[1;32m[%s] PASS\033[0m %s\n' "$(date +%H:%M:%S)" "$*" >&2; }
fail() { printf '\033[1;31m[%s] FAIL\033[0m %s\n' "$(date +%H:%M:%S)" "$*" >&2; exit 1; }

TEST_DIR=$(mktemp -d -t osvbng-survival-XXXXXX)
TOPO_BAK=""

cleanup() {
    local rc=$?
    log "Cleanup (rc=$rc)"
    if [[ "${OSVBNG_KEEP_TOPO:-0}" != "1" ]]; then
        sudo containerlab destroy -t "$TOPO" --cleanup >/dev/null 2>&1 || true
        sudo docker rm -f "${BNG}" "${SUBS}" "clab-${LAB_NAME}-corerouter1" >/dev/null 2>&1 || true
    else
        log "preserving topology — clean up with: sudo containerlab destroy -t $TOPO --cleanup"
    fi
    [[ -n "$TOPO_BAK" && -f "$TOPO_BAK" ]] && mv "$TOPO_BAK" "$TOPO"
    if [[ "${OSVBNG_KEEP_STATE:-0}" != "1" && $rc -eq 0 ]]; then
        rm -rf "$TEST_DIR"
    else
        log "preserved state at $TEST_DIR"
    fi
}
trap cleanup EXIT

# ---------------------------------------------------------------- helpers
ssh_bng()  { sudo docker exec "$BNG" ssh -i /root/.ssh/id_ed25519 \
                -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
                -o LogLevel=ERROR -p 2022 root@127.0.0.1 -- "$@"; }
# Pipe a host file into the VM by way of the wrapper container.
# Plain `docker cp` only reaches the wrapper, not the inner VM.
scp_bng()  { sudo docker exec -i "$BNG" ssh -i /root/.ssh/id_ed25519 \
                -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
                -o LogLevel=ERROR -p 2022 root@127.0.0.1 -- "cat > $2" < "$1"; }
api_count() {
    { ssh_bng 'curl -sf http://localhost:8080/api/show/subscriber/sessions' 2>/dev/null \
        || echo '{}'; } | python3 -c 'import sys,json
try: print(len((json.load(sys.stdin).get("data") or [])))
except Exception: print(0)'
}
blaster_established() {
    { sudo docker exec "$SUBS" bngblaster-cli /run/bngblaster.sock session-counters 2>/dev/null \
        || echo '{}'; } | python3 -c 'import sys,json
try: print(json.load(sys.stdin).get("session-counters",{}).get("sessions-established",0))
except Exception: print(0)'
}

# ----------------------------------------------------------- 1. test keys
log "Generating per-run cosign keypair"
( cd "$TEST_DIR" && COSIGN_PASSWORD="" cosign generate-key-pair \
        --output-key-prefix test-cosign >/dev/null )
chmod 0600 "$TEST_DIR/test-cosign.key"

# ----------------------------------------------------------- 2. clab deploy
log "Rewriting topology for --qemu (cosign trust-anchor swapped after boot)"
TAG=$(sudo docker images --format '{{.Tag}}' veesixnetworks/osvbng \
        | awk '/^ci-v/ {print; exit}')
[[ -z "$TAG" ]] && fail "no veesixnetworks/osvbng:ci-v* image found (run scripts/qemu/vrnetlab/build.sh first)"
TOPO_BAK="$TOPO.survival-bak"
cp "$TOPO" "$TOPO_BAK"
sed -i \
    -e "s|veesixnetworks/osvbng:local|veesixnetworks/osvbng:$TAG|g" \
    -e "/image: veesixnetworks\\/osvbng:$TAG/a\\      devices: [\"/dev/kvm\"]" \
    -e "/^    - endpoints:/a\\      mtu: 1500" \
    "$TOPO"

log "Deploying clab topology"
sudo containerlab deploy --reconfigure -t "$TOPO" >/dev/null

# ---------------------------------------------------------- 3. wait healthy
log "Waiting for osvbngd state=ready inside BNG VM"
for i in $(seq 1 60); do
    state=$(ssh_bng 'cat /run/osvbng/state 2>/dev/null' 2>/dev/null || true)
    [[ "$state" == *'"state":"ready"'* ]] && { ok "BNG ready after ${i}0s"; break; }
    sleep 5
done
[[ "$state" == *'"state":"ready"'* ]] || fail "BNG never reached state=ready"

# ----------------------------------------------- 4. swap cosign trust anchor
log "Hot-swapping baked-in cosign.pub with this run's test public key"
scp_bng "$TEST_DIR/test-cosign.pub" "/tmp/test-cosign.pub"
ssh_bng 'cp /tmp/test-cosign.pub /etc/osvbng/release-keys/cosign.pub'
ssh_bng 'rm -rf /var/opt/osvbng/upgrade-journal/* 2>/dev/null; true'

# ----------------------------------------------- 4b. provision local-auth users
# 03's bngblaster sends agent-remote-id=DEV-<session-global>. The local
# auth provider runs with allow_all=false so the matching DEV-N user
# must exist before bngblaster starts or every DHCP exchange returns
# allowed=false.
log "Provisioning local-auth user DEV-1"
ssh_bng "curl -sf -X POST http://localhost:8080/api/exec/subscriber/auth/local/users/create -H 'Content-Type: application/json' -d '{\"username\":\"DEV-1\",\"enabled\":true}'" >/dev/null \
    || fail "could not provision DEV-1 user"

# ----------------------------------------------- 5. start subscriber traffic
log "Starting bngblaster (IPoE)"
sudo docker exec -d "$SUBS" bngblaster -C /config/config.json \
    -J /tmp/report.json -L /tmp/bngblaster.log -S /run/bngblaster.sock -b -f

log "Waiting for $EXPECTED_SESSIONS subscriber session(s) to establish (up to 180s)"
for i in $(seq 1 90); do
    est=$(blaster_established)
    [[ "$est" -ge "$EXPECTED_SESSIONS" ]] && break
    sleep 2
done
[[ "$est" -ge "$EXPECTED_SESSIONS" ]] || fail "bngblaster only established $est/$EXPECTED_SESSIONS sessions pre-upgrade"
ok "Pre-upgrade: $est session(s) established"

api_pre=$(api_count)
[[ "$api_pre" -ge "$EXPECTED_SESSIONS" ]] || fail "BNG API sees $api_pre sessions, want >= $EXPECTED_SESSIONS"
ok "Pre-upgrade: BNG API reports $api_pre session(s)"

# ----------------------------------------------------- 6. build + scp tarball
log "Building v$TARGET_VERSION tarball signed with test key"
COSIGN_PASSWORD="" "$REPO_ROOT/scripts/release/build-test-tarball.sh" \
    -k "$TEST_DIR/test-cosign.key" -v "$TARGET_VERSION" -o "$TEST_DIR" >/dev/null
TARBALL="$TEST_DIR/osvbng-v${TARGET_VERSION}.tar.gz"
[[ -f "$TARBALL" ]] || fail "tarball not produced at $TARBALL"
scp_bng "$TARBALL" "/tmp/osvbng-v${TARGET_VERSION}.tar.gz"
scp_bng "${TARBALL}.sig" "/tmp/osvbng-v${TARGET_VERSION}.tar.gz.sig"

# ---------------------------------------------------------------- 7. apply
log "Triggering upgrade apply (measuring API-visible session outage)"
UPGRADE_START=$(date +%s)
ssh_bng "echo 'upgrade apply --force-retry /tmp/osvbng-v${TARGET_VERSION}.tar.gz' | osvbngcli" \
    > "$TEST_DIR/apply.log" 2>&1 &
APPLY_PID=$!

# Sample the session count every 1s while upgrade runs.
> "$TEST_DIR/outage.log"
loss_started=""
loss_ended=""
while kill -0 "$APPLY_PID" 2>/dev/null; do
    now=$(date +%s)
    c=$(api_count)
    printf '%s sessions=%s\n' "$now" "$c" >> "$TEST_DIR/outage.log"
    if [[ "$c" -lt "$EXPECTED_SESSIONS" && -z "$loss_started" ]]; then
        loss_started=$now
        log "Session dropped at +$((now-UPGRADE_START))s"
    fi
    if [[ -n "$loss_started" && -z "$loss_ended" && "$c" -ge "$EXPECTED_SESSIONS" ]]; then
        loss_ended=$now
        log "Session restored at +$((now-UPGRADE_START))s"
    fi
    sleep 1
done
wait "$APPLY_PID" || true
UPGRADE_END=$(date +%s)
log "upgrade apply returned after $((UPGRADE_END-UPGRADE_START))s"

# ------------------------------------------------------- 8. post-upgrade asserts
log "Verifying journal phase + version"
journal=$(ssh_bng "cat /var/opt/osvbng/upgrade-state.json 2>/dev/null" || true)
echo "$journal" | grep -qE '"phase"[[:space:]]*:[[:space:]]*"completed"' \
    || fail "journal phase != completed: $journal"
ok "journal phase == completed"
manifest_ver=$(ssh_bng "awk '/^osvbng_version:/ {print \$2}' /var/opt/osvbng/current-manifest.yaml 2>/dev/null" || true)
[[ "$manifest_ver" == "$TARGET_VERSION" ]] \
    || fail "current-manifest osvbng_version=$manifest_ver want $TARGET_VERSION"
ok "current-manifest osvbng_version == $TARGET_VERSION"
ver=$(ssh_bng "osvbngd --version 2>&1" || true)
echo "$ver" | grep -q "$TARGET_VERSION" \
    || fail "osvbngd --version missing $TARGET_VERSION: $ver"
ok "osvbngd --version contains $TARGET_VERSION"

log "Waiting up to ${RECOVERY_BUDGET_S}s for sessions to fully re-establish"
deadline=$((UPGRADE_END + RECOVERY_BUDGET_S))
while (( $(date +%s) < deadline )); do
    api_post=$(api_count)
    est_post=$(blaster_established)
    [[ "$api_post" -ge "$EXPECTED_SESSIONS" && "$est_post" -ge "$EXPECTED_SESSIONS" ]] && break
    sleep 2
done
[[ "$api_post" -ge "$EXPECTED_SESSIONS" ]] \
    || fail "BNG API sees $api_post/$EXPECTED_SESSIONS sessions after ${RECOVERY_BUDGET_S}s"
[[ "$est_post" -ge "$EXPECTED_SESSIONS" ]] \
    || fail "bngblaster sees $est_post/$EXPECTED_SESSIONS sessions after ${RECOVERY_BUDGET_S}s"
ok "Post-upgrade: BNG API $api_post, bngblaster $est_post sessions"

# --------------------------------------------------------- 9. outage summary
if [[ -n "$loss_started" && -n "$loss_ended" ]]; then
    outage=$((loss_ended - loss_started))
    log "Observed session-API outage: ${outage}s"
elif [[ -z "$loss_started" ]]; then
    log "No session-API outage observed (state restore worked end-to-end)"
else
    log "Session outage started at +$((loss_started-UPGRADE_START))s and had not recovered by upgrade exit"
fi

ok "05-session-survival-ipoe"
