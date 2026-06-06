# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Sourced by every scenario under tests/41-upgrade-tier-a-v2. Owns VM
# lifecycle (cloud-init seed, qemu spawn, SSH wait, scp/exec, shutdown)
# and key-material setup (test cosign + SSH keypair, both regenerated
# per-run under a tmpdir so multiple runs don't collide).

set -euo pipefail

if [[ -z "${REPO_ROOT:-}" ]]; then
    REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
fi

BASE_IMG_DEFAULT=/var/lib/libvirt/images/osvbng.qcow2
BASE_IMG="${OSVBNG_BASE_IMG:-$BASE_IMG_DEFAULT}"
# RAM default is intentionally large: the packer image's GRUB reserves
# most of physical RAM for 1G hugepages at boot, so a small VM OOMs in
# kthreadd before sshd ever starts. 8 GiB gives the kernel comfortable
# headroom under that reservation.
VM_RAM_MB="${OSVBNG_VM_RAM:-8192}"
VM_CPUS="${OSVBNG_VM_CPUS:-2}"
SSH_USER="${OSVBNG_SSH_USER:-root}"
SSH_TIMEOUT_SECONDS="${OSVBNG_SSH_TIMEOUT:-180}"

# All per-run state lives under TEST_DIR. Caller sets it; we default to
# a fresh /tmp dir so trap-on-EXIT cleanup nukes the whole tree.
if [[ -z "${TEST_DIR:-}" ]]; then
    TEST_DIR="$(mktemp -d /tmp/osvbng-upgrade-test.XXXXXX)"
    export TEST_DIR
fi

log()  { printf '\033[1;34m[%s]\033[0m %s\n' "$(date +%H:%M:%S)" "$*" >&2; }
ok()   { printf '\033[1;32m[%s] PASS\033[0m %s\n' "$(date +%H:%M:%S)" "$*" >&2; }
fail() { printf '\033[1;31m[%s] FAIL\033[0m %s\n' "$(date +%H:%M:%S)" "$*" >&2; exit 1; }

pick_free_port() {
    # Ports 22000–22999 reserved for these tests so parallel runs are
    # rarer than picking from 1024–65535. Caller still races on bind,
    # but qemu fails loudly if the port is taken.
    while :; do
        port=$(( 22000 + RANDOM % 1000 ))
        if ! ss -tln 2>/dev/null | awk '{print $4}' | grep -q ":$port\$"; then
            echo "$port"
            return
        fi
    done
}

setup_keys() {
    log "Generating test SSH + cosign keypairs under $TEST_DIR"
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
    if [[ ! -f "$BASE_IMG" ]]; then
        fail "Base qcow2 missing at $BASE_IMG. Set OSVBNG_BASE_IMG or fetch via scripts/release/build-test-tarball.sh"
    fi

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
        fail "qemu refused to start (see above)"
    fi
}

ssh_opts() {
    echo "-i $TEST_DIR/id_ed25519 -p $SSH_PORT -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ConnectTimeout=5"
}

scp_opts() {
    # scp uses -P for port; ssh uses -p. Same -i / -o flags otherwise.
    echo "-i $TEST_DIR/id_ed25519 -P $SSH_PORT -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ConnectTimeout=5"
}

wait_for_ssh() {
    log "Waiting up to ${SSH_TIMEOUT_SECONDS}s for sshd"
    local deadline
    deadline=$(( $(date +%s) + SSH_TIMEOUT_SECONDS ))
    while [[ $(date +%s) -lt $deadline ]]; do
        if ssh $(ssh_opts) "${SSH_USER}@127.0.0.1" -- true 2>/dev/null; then
            ok "ssh up"
            return
        fi
        sleep 3
    done
    fail "sshd never reached (see $TEST_DIR/serial.log)"
}

vm_ssh() {
    ssh $(ssh_opts) "${SSH_USER}@127.0.0.1" -- "$@"
}

vm_ssh_in() {
    # Reads commands from stdin. Used for multi-line heredoc scripts.
    ssh $(ssh_opts) "${SSH_USER}@127.0.0.1" -- bash -se
}

vm_scp_to() {
    local src="$1"
    local dst="$2"
    scp $(scp_opts) -q "$src" "${SSH_USER}@127.0.0.1:$dst"
}

shutdown_vm() {
    if [[ -f "$TEST_DIR/qemu.pid" ]]; then
        local pid
        pid="$(cat "$TEST_DIR/qemu.pid")"
        if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
            log "Shutting down qemu pid=$pid"
            kill "$pid" 2>/dev/null || true
            for _ in $(seq 1 10); do
                kill -0 "$pid" 2>/dev/null || break
                sleep 1
            done
            kill -9 "$pid" 2>/dev/null || true
        fi
    fi
}

trap_cleanup() {
    local rc=$?
    shutdown_vm
    if [[ "$rc" -ne 0 ]]; then
        log "Test failed (rc=$rc). State preserved at $TEST_DIR for forensics."
        log "Serial console: $TEST_DIR/serial.log"
    else
        if [[ -n "${OSVBNG_KEEP_STATE:-}" ]]; then
            log "OSVBNG_KEEP_STATE set; preserving $TEST_DIR"
        else
            rm -rf "$TEST_DIR"
        fi
    fi
}

# Cross-compile v2 binaries on the host and scp them onto the VM,
# replacing the v0.13.0 install. Also drops the test cosign.pub so
# tarballs we build can be verified by the on-VM runner, and clears
# the v0.13.0 current-manifest.yaml (its v1 schema would refuse to
# parse under our v2 builds — CurrentInstalledVersion's binary-version
# fallback takes over when the file is absent).
bootstrap_v2() {
    log "Replacing /etc/osvbng/osvbng.yaml with minimal-config fixture"
    # The image's osvbng.yaml binds eth1 via DPDK at a hardcoded PCI BDF
    # that doesn't exist in QEMU user-mode networking. osvbngd refuses to
    # come ready when the bind fails. The minimal config has no
    # interfaces / subscriber-groups, exercising just the supervisory
    # startup the upgrade flow needs.
    vm_scp_to "$REPO_ROOT/tests/41-upgrade-tier-a-v2/fixtures/minimal-osvbng.yaml" /tmp/osvbng.yaml
    vm_ssh "mv /tmp/osvbng.yaml /etc/osvbng/osvbng.yaml"

    log "Installing RuntimeDirectoryPreserve=yes drop-in on osvbng.service"
    # The shipped osvbng.service declares RuntimeDirectory=osvbng. systemd
    # removes /run/osvbng/ on every stop, taking vpp's dataplane_api.sock
    # with it; on next start, vpp.service is still active but its socket
    # file is gone, so osvbngd fails its connect-to-VPP check. Preserve
    # the dir across stops so a binary-swap restart leaves the socket
    # untouched. This is independent of the upgrade work and would be a
    # useful upstream fix to the packer image's systemd unit.
    vm_ssh_in <<'PRESERVE'
set -e
mkdir -p /etc/systemd/system/osvbng.service.d
cat > /etc/systemd/system/osvbng.service.d/runtime-preserve.conf <<'EOF'
[Service]
RuntimeDirectoryPreserve=yes
EOF
systemctl daemon-reload
PRESERVE

    log "Restarting vpp.service to recreate the dataplane_api.sock that prior osvbngd churn destroyed"
    vm_ssh "systemctl restart vpp.service" || fail "vpp restart failed"
    local vpp_deadline=$(( $(date +%s) + 90 ))
    while [[ $(date +%s) -lt $vpp_deadline ]]; do
        if vm_ssh "systemctl is-active --quiet vpp.service && test -S /run/osvbng/dataplane_api.sock" 2>/dev/null; then
            ok "vpp.service active and dataplane_api.sock present"
            break
        fi
        sleep 2
    done
    if ! vm_ssh "test -S /run/osvbng/dataplane_api.sock" 2>/dev/null; then
        vm_ssh "systemctl status vpp.service --no-pager -l || true" || true
        fail "vpp.service socket never came back"
    fi

    log "Building v2 binaries on host"
    (cd "$REPO_ROOT" && VERSION=0.14.0-test make build >/dev/null)

    log "Staging binaries + cosign.pub onto VM"
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

    log "Waiting for osvbng to reach state=ready"
    local deadline=$(( $(date +%s) + 60 ))
    while [[ $(date +%s) -lt $deadline ]]; do
        if vm_ssh "test -f /run/osvbng/state && grep -q '\"state\":\"ready\"' /run/osvbng/state" 2>/dev/null; then
            ok "osvbng ready"
            return
        fi
        sleep 2
    done
    log "osvbng state never reached ready. Diagnostics:"
    vm_ssh "systemctl status osvbng.service --no-pager -l || true" || true
    vm_ssh "journalctl -u osvbng.service --no-pager -n 80 || true" || true
    vm_ssh "cat /run/osvbng/state 2>/dev/null || echo '<state file absent>'" || true
    fail "osvbng never reached state=ready after bootstrap"
}

# Build a signed test tarball locally and scp it into the VM.
# Args: target_version, output_var_name
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
    vm_scp_to "$tar"       "/var/lib/osvbng-test/$(basename "$tar")"
    vm_scp_to "$tar.sig"   "/var/lib/osvbng-test/$(basename "$tar").sig"

    eval "$out_var=/var/lib/osvbng-test/$(basename "$tar")"
}
