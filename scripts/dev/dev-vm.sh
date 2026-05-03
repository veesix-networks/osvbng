#!/bin/bash
# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Bootstrap a dev VM with virsh/KVM.
# Downloads a Debian 12 cloud image, creates a VM, and provisions it.
#
# Usage: ./dev-vm.sh [path/to/key.pub]
#
# After creation, use virsh directly to manage the VM:
#   virsh start osvbng-dev
#   virsh shutdown osvbng-dev
#   virsh destroy osvbng-dev    # force stop
#   virsh undefine osvbng-dev --remove-all-storage

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

VM_NAME="osvbng-dev"
VM_MEMORY="${VM_MEMORY:-6144}"
VM_VCPUS=4
VM_DISK_SIZE="30G"
LIBVIRT_URI="qemu:///system"

DATA_DIR="/var/lib/libvirt/images/osvbng-dev"
DISK_PATH="$DATA_DIR/${VM_NAME}.qcow2"
CLOUD_INIT_ISO="$DATA_DIR/cloud-init.iso"
DEBIAN_IMAGE_URL="https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-generic-amd64.qcow2"
DEBIAN_IMAGE_CACHE="$DATA_DIR/debian-12-generic-amd64.qcow2"

SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR"

preflight_checks() {
    for cmd in virsh virt-install qemu-img cloud-localds; do
        command -v "$cmd" &>/dev/null || {
            echo "Missing: $cmd. Install with: sudo apt install virtinst libvirt-daemon-system cloud-image-utils qemu-utils"
            exit 1
        }
    done

    if command -v whiptail &>/dev/null; then
        TUI_TOOL="whiptail"
    elif command -v dialog &>/dev/null; then
        TUI_TOOL="dialog"
    else
        echo "Error: Neither whiptail nor dialog found."
        echo "  Debian/Ubuntu: sudo apt install whiptail"
        echo "  Or:            sudo apt install dialog"
        exit 1
    fi

    VIRT_TYPE="kvm"
    if [ ! -w /dev/kvm ]; then
        echo "WARNING: KVM not available. VM will run under emulation and be very slow."
        VIRT_TYPE="qemu"
    fi

    if virsh --connect "$LIBVIRT_URI" list --all --name 2>/dev/null | grep -qx "$VM_NAME"; then
        echo "VM '$VM_NAME' already exists. To recreate:"
        echo "  virsh destroy $VM_NAME; virsh undefine $VM_NAME --remove-all-storage"
        exit 1
    fi
}

select_ssh_keys() {
    PUB_KEYS=()
    for keyfile in "$HOME"/.ssh/*.pub; do
        [ -f "$keyfile" ] && PUB_KEYS+=("$keyfile")
    done
    [ ${#PUB_KEYS[@]} -gt 0 ] || { echo "No SSH public keys found in ~/.ssh/. Run: ssh-keygen"; exit 1; }

    if [ ${#PUB_KEYS[@]} -eq 1 ]; then
        SELECTED_KEYS=("${PUB_KEYS[0]}")
    else
        CHECKLIST_ARGS=()
        for keyfile in "${PUB_KEYS[@]}"; do
            CHECKLIST_ARGS+=("$keyfile" "$(basename "$keyfile")" "ON")
        done

        SELECTED=$($TUI_TOOL --title "SSH Key Selection" --checklist \
            "Select SSH public key(s) to copy into the dev VM:" 15 70 ${#PUB_KEYS[@]} \
            "${CHECKLIST_ARGS[@]}" 3>&1 1>&2 2>&3) || { echo "Cancelled."; exit 1; }

        SELECTED_KEYS=()
        for item in $SELECTED; do
            SELECTED_KEYS+=("${item//\"/}")
        done
        [ ${#SELECTED_KEYS[@]} -gt 0 ] || { echo "No keys selected."; exit 1; }
    fi

    SSH_KEYS=""
    for keyfile in "${SELECTED_KEYS[@]}"; do
        SSH_KEYS="${SSH_KEYS}      - $(cat "$keyfile")"$'\n'
    done
}

prepare_disk() {
    sudo mkdir -p "$DATA_DIR"

    if [ ! -f "$DEBIAN_IMAGE_CACHE" ]; then
        echo "Downloading Debian 12 cloud image..."
        sudo curl -fL -o "$DEBIAN_IMAGE_CACHE" "$DEBIAN_IMAGE_URL"
    fi

    sudo cp "$DEBIAN_IMAGE_CACHE" "$DISK_PATH"
    sudo qemu-img resize "$DISK_PATH" "$VM_DISK_SIZE"
}

create_cloud_init() {
    USERDATA=$(mktemp)
    cat > "$USERDATA" <<EOF
#cloud-config
hostname: ${VM_NAME}
users:
  - name: dev
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    groups: sudo
    ssh_authorized_keys:
${SSH_KEYS}
package_update: false
runcmd:
  - systemctl enable --now qemu-guest-agent || true
EOF
    sudo cloud-localds "$CLOUD_INIT_ISO" "$USERDATA"
    rm -f "$USERDATA"
}

create_vm() {
    echo "Creating VM..."
    virt-install \
        --connect "$LIBVIRT_URI" \
        --name "$VM_NAME" \
        --memory "$VM_MEMORY" \
        --vcpus "$VM_VCPUS" \
        --virt-type "$VIRT_TYPE" \
        --cpu host-passthrough \
        --os-variant debian12 \
        --import \
        --disk "path=${DISK_PATH},format=qcow2,bus=virtio" \
        --disk "path=${CLOUD_INIT_ISO},device=cdrom" \
        --network network=default,model=virtio \
        --graphics none \
        --noautoconsole \
        --quiet
}

wait_for_vm() {
    echo "Waiting for VM..."
    IP=""
    for i in $(seq 1 40); do
        IP=$(virsh --connect "$LIBVIRT_URI" domifaddr "$VM_NAME" 2>/dev/null \
            | awk '/ipv4/ {split($4, a, "/"); print a[1]; exit}')
        [ -n "$IP" ] && break
        sleep 3
    done
    [ -n "$IP" ] || { echo "Timed out waiting for VM IP"; exit 1; }

    for i in $(seq 1 40); do
        ssh $SSH_OPTS -o ConnectTimeout=3 "dev@${IP}" true 2>/dev/null && break
        sleep 3
    done
}

provision_vm() {
    echo "Provisioning..."
    . "$PROJECT_ROOT/versions.env"
    scp $SSH_OPTS "$SCRIPT_DIR/provision.sh" "dev@${IP}:/tmp/provision.sh"
    ssh $SSH_OPTS "dev@${IP}" "sudo env GOLANG_VERSION=${GOLANG_VERSION} DATAPLANE_VERSION=${DATAPLANE_VERSION} CONTAINERLAB_VERSION=${CONTAINERLAB_VERSION} bash /tmp/provision.sh"
}

# --- Main ---

preflight_checks
select_ssh_keys
prepare_disk
create_cloud_init
create_vm
wait_for_vm
provision_vm

echo ""
echo "Dev VM ready! IP: ${IP}"
echo ""
echo "  ssh dev@${IP}"
echo ""
echo "Add to ~/.ssh/config for VSCode Remote SSH:"
echo ""
echo "  Host ${VM_NAME}"
echo "    HostName ${IP}"
echo "    User dev"
echo ""
