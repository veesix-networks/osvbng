#!/bin/bash
# Copyright 2025 Veesix Networks Ltd
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Re-run provisioning on the dev VM.
# Useful after updating versions.env or provision.sh.
# Usage: ./reprovision-vm.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

VM_NAME="osvbng-dev"
LIBVIRT_URI="qemu:///system"
SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR"

IP=$(virsh --connect "$LIBVIRT_URI" domifaddr "$VM_NAME" 2>/dev/null \
    | awk '/ipv4/ {split($4, a, "/"); print a[1]; exit}')
[ -n "$IP" ] || { echo "Could not determine VM IP. Is the VM running?"; exit 1; }

# Sync the latest provision script and versions
scp $SSH_OPTS "$SCRIPT_DIR/provision.sh" "dev@${IP}:/tmp/provision.sh"

. "$PROJECT_ROOT/versions.env"
ssh $SSH_OPTS "dev@${IP}" "sudo env GOLANG_VERSION=${GOLANG_VERSION} DATAPLANE_VERSION=${DATAPLANE_VERSION} CONTAINERLAB_VERSION=${CONTAINERLAB_VERSION} bash /tmp/provision.sh"
