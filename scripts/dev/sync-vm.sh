#!/bin/bash
# Copyright 2025 Veesix Networks Ltd
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Rsync project into the dev VM.
# Usage: ./sync-vm.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

VM_NAME="osvbng-dev"
LIBVIRT_URI="qemu:///system"
SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR"

IP=$(virsh --connect "$LIBVIRT_URI" domifaddr "$VM_NAME" 2>/dev/null \
    | awk '/ipv4/ {split($4, a, "/"); print a[1]; exit}')
[ -n "$IP" ] || { echo "Could not determine VM IP. Is the VM running?"; exit 1; }

rsync -avz --delete \
    --exclude .git \
    --exclude bin/ \
    --exclude output/ \
    --exclude '*.qcow2*' \
    -e "ssh $SSH_OPTS" \
    "$PROJECT_ROOT/" "dev@${IP}:~/osvbng/"
