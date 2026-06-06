# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

set -euo pipefail

if [[ -z "${REPO_ROOT:-}" ]]; then
    REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
fi

BASE_IMG="${OSVBNG_BASE_IMG:-/var/lib/libvirt/images/osvbng.qcow2}"
# 8 GiB clears the packer image's 1G-hugepage GRUB reservation; smaller
# values OOM at boot.
VM_RAM_MB="${OSVBNG_VM_RAM:-8192}"
VM_CPUS="${OSVBNG_VM_CPUS:-2}"
SSH_USER="${OSVBNG_SSH_USER:-root}"
SSH_TIMEOUT_SECONDS="${OSVBNG_SSH_TIMEOUT:-180}"

if [[ -z "${TEST_DIR:-}" ]]; then
    TEST_DIR="$(mktemp -d /tmp/osvbng-upgrade-test.XXXXXX)"
    export TEST_DIR
fi

log()  { printf '\033[1;34m[%s]\033[0m %s\n' "$(date +%H:%M:%S)" "$*" >&2; }
ok()   { printf '\033[1;32m[%s] PASS\033[0m %s\n' "$(date +%H:%M:%S)" "$*" >&2; }
fail() { printf '\033[1;31m[%s] FAIL\033[0m %s\n' "$(date +%H:%M:%S)" "$*" >&2; exit 1; }

pick_free_port() {
    while :; do
        port=$(( 22000 + RANDOM % 1000 ))
        if ! ss -tln 2>/dev/null | awk '{print $4}' | grep -q ":$port\$"; then
            echo "$port"
            return
        fi
    done
}

setup_keys() {
    log "Generating test SSH + cosign keypairs"
    ssh-keygen -t ed25519 -N '' -f "$TEST_DIR/id_ed25519" -C "osvbng-upgrade-test" -q
    (
        cd "$TEST_DIR"
        COSIGN_PASSWORD="" cosign generate-key-pair --output-key-prefix test-cosign >/dev/null
    )
    chmod 0600 "$TEST_DIR/test-cosign.key"
}

build_cloud_init_seed() {
    log "Building cloud-init seed ISO"
    cat > "$TEST_DIR/user-data" <<EOF
#cloud-config
ssh_authorized_keys:
  - $(cat "$TEST_DIR/id_ed25519.pub")
disable_root: false
ssh_pwauth: false
package_update: false
EOF
    cat > "$TEST_DIR/meta-data" <<EOF
instance-id: osvbng-upgrade-test
local-hostname: osvbng-test
EOF
    xorrisofs \
        -quiet \
        -output "$TEST_DIR/seed.iso" \
        -volid CIDATA \
        -joliet -rock \
        "$TEST_DIR/user-data" "$TEST_DIR/meta-data" \
        > /dev/null 2>&1
}

boot_vm() {
    [[ -f "$BASE_IMG" ]] || fail "Base qcow2 missing at $BASE_IMG"

    log "Creating CoW overlay of $BASE_IMG"
    qemu-img create -q -f qcow2 -F qcow2 -b "$BASE_IMG" "$TEST_DIR/test.qcow2" 40G > /dev/null

    SSH_PORT="$(pick_free_port)"
    export SSH_PORT

    log "Booting VM (RAM=${VM_RAM_MB} MB, vCPUs=$VM_CPUS, SSH on localhost:$SSH_PORT)"
    qemu-system-x86_64 \
        -enable-kvm \
        -m "$VM_RAM_MB" \
        -smp "$VM_CPUS" \
        -cpu host \
        -display none \
        -serial "file:$TEST_DIR/serial.log" \
        -drive "file=$TEST_DIR/test.qcow2,if=virtio,format=qcow2" \
        -drive "file=$TEST_DIR/seed.iso,if=virtio,format=raw,readonly=on" \
        -netdev "user,id=net0,hostfwd=tcp::$SSH_PORT-:22" \
        -device "virtio-net-pci,netdev=net0" \
        -pidfile "$TEST_DIR/qemu.pid" \
        -daemonize \
        > "$TEST_DIR/qemu.out" 2>&1

    if [[ ! -s "$TEST_DIR/qemu.pid" ]]; then
        cat "$TEST_DIR/qemu.out" >&2
        fail "qemu refused to start"
    fi
}

# scp uses -P; ssh uses -p.
ssh_opts() { echo "-i $TEST_DIR/id_ed25519 -p $SSH_PORT -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ConnectTimeout=5"; }
scp_opts() { echo "-i $TEST_DIR/id_ed25519 -P $SSH_PORT -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ConnectTimeout=5"; }

wait_for_ssh() {
    log "Waiting up to ${SSH_TIMEOUT_SECONDS}s for sshd"
    local deadline=$(( $(date +%s) + SSH_TIMEOUT_SECONDS ))
    while [[ $(date +%s) -lt $deadline ]]; do
        if ssh $(ssh_opts) "${SSH_USER}@127.0.0.1" -- true 2>/dev/null; then
            ok "ssh up"
            return
        fi
        sleep 3
    done
    fail "sshd never reached (see $TEST_DIR/serial.log)"
}

vm_ssh()    { ssh $(ssh_opts) "${SSH_USER}@127.0.0.1" -- "$@"; }
vm_ssh_in() { ssh $(ssh_opts) "${SSH_USER}@127.0.0.1" -- bash -se; }
vm_scp_to() { scp $(scp_opts) -q "$1" "${SSH_USER}@127.0.0.1:$2"; }

shutdown_vm() {
    [[ -f "$TEST_DIR/qemu.pid" ]] || return 0
    local pid
    pid="$(cat "$TEST_DIR/qemu.pid")"
    [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null || return 0
    log "Shutting down qemu pid=$pid"
    kill "$pid" 2>/dev/null || true
    for _ in $(seq 1 10); do
        kill -0 "$pid" 2>/dev/null || return 0
        sleep 1
    done
    kill -9 "$pid" 2>/dev/null || true
}

trap_cleanup() {
    local rc=$?
    shutdown_vm
    if [[ "$rc" -ne 0 ]]; then
        log "Test failed (rc=$rc). State preserved at $TEST_DIR for forensics."
        log "Serial console: $TEST_DIR/serial.log"
    elif [[ -z "${OSVBNG_KEEP_STATE:-}" ]]; then
        rm -rf "$TEST_DIR"
    fi
}

bootstrap_v2() {
    log "Installing minimal osvbng.yaml fixture"
    vm_scp_to "$REPO_ROOT/tests/41-upgrade-tier-a-v2/fixtures/minimal-osvbng.yaml" /tmp/osvbng.yaml
    vm_ssh "mv /tmp/osvbng.yaml /etc/osvbng/osvbng.yaml"

    # systemctl stop nukes RuntimeDirectory=osvbng, taking vpp's
    # dataplane_api.sock with it. Workaround until osvbng-context#164 lands.
    log "Installing RuntimeDirectoryPreserve=yes drop-in"
    vm_ssh_in <<'PRESERVE'
set -e
mkdir -p /etc/systemd/system/osvbng.service.d
cat > /etc/systemd/system/osvbng.service.d/runtime-preserve.conf <<'EOF'
[Service]
RuntimeDirectoryPreserve=yes
EOF
systemctl daemon-reload
PRESERVE

    log "Restarting vpp.service to recreate the api socket"
    vm_ssh "systemctl restart vpp.service" || fail "vpp restart failed"
    local vpp_deadline=$(( $(date +%s) + 90 ))
    while [[ $(date +%s) -lt $vpp_deadline ]]; do
        vm_ssh "systemctl is-active --quiet vpp.service && test -S /run/osvbng/dataplane_api.sock" 2>/dev/null && { ok "vpp ready"; break; }
        sleep 2
    done
    vm_ssh "test -S /run/osvbng/dataplane_api.sock" 2>/dev/null || fail "vpp api socket never came back"

    log "Building v2 binaries"
    (cd "$REPO_ROOT" && VERSION=0.14.0-test make build >/dev/null)

    log "Staging binaries + cosign.pub"
    vm_scp_to "$REPO_ROOT/bin/osvbngd" /tmp/osvbngd
    vm_scp_to "$REPO_ROOT/bin/osvbngcli" /tmp/osvbngcli
    vm_scp_to "$TEST_DIR/test-cosign.pub" /tmp/test-cosign.pub

    vm_ssh_in <<'BOOTSTRAP'
set -e
systemctl stop osvbng.service 2>/dev/null || true
mv /tmp/osvbngd  /usr/local/bin/osvbngd
mv /tmp/osvbngcli /usr/local/bin/osvbngcli
chmod 0755 /usr/local/bin/osvbngd /usr/local/bin/osvbngcli
cp /tmp/test-cosign.pub /etc/osvbng/release-keys/cosign.pub
rm -f /var/opt/osvbng/current-manifest.yaml /var/opt/osvbng/upgrade-state.json
systemctl start osvbng.service
BOOTSTRAP

    log "Waiting for osvbng state=ready"
    local deadline=$(( $(date +%s) + 60 ))
    while [[ $(date +%s) -lt $deadline ]]; do
        vm_ssh "test -f /run/osvbng/state && grep -q '\"state\":\"ready\"' /run/osvbng/state" 2>/dev/null && { ok "osvbng ready"; return; }
        sleep 2
    done
    vm_ssh "systemctl status osvbng.service --no-pager -l || true" || true
    vm_ssh "journalctl -u osvbng.service --no-pager -n 80 || true" || true
    vm_ssh "cat /run/osvbng/state 2>/dev/null || echo '<state file absent>'" || true
    fail "osvbng never reached state=ready"
}

build_and_push_tarball() {
    local version="$1"
    local out_var="$2"
    local outdir="$TEST_DIR/tarballs"
    mkdir -p "$outdir"

    log "Building signed v2 tarball v$version"
    (
        cd "$REPO_ROOT"
        COSIGN_PASSWORD="" ./scripts/release/build-test-tarball.sh \
            -k "$TEST_DIR/test-cosign.key" \
            -v "$version" \
            -o "$outdir" \
            > "$TEST_DIR/build-tarball-$version.log" 2>&1
    )

    local tar="$outdir/osvbng-v$version.tar.gz"
    [[ -f "$tar" ]] || { cat "$TEST_DIR/build-tarball-$version.log" >&2; fail "tarball build failed"; }

    log "Pushing tarball to VM"
    vm_ssh "mkdir -p /var/lib/osvbng-test"
    vm_scp_to "$tar"     "/var/lib/osvbng-test/$(basename "$tar")"
    vm_scp_to "$tar.sig" "/var/lib/osvbng-test/$(basename "$tar").sig"

    eval "$out_var=/var/lib/osvbng-test/$(basename "$tar")"
}
