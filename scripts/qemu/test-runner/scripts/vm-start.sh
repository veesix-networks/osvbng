#!/bin/bash
# Copyright 2025 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

set -euo pipefail

QCOW2="${1:?Usage: vm-start.sh <qcow2> [ssh-port]}"
SSH_PORT="${2:-2222}"

OVERLAY="/tmp/test-runner-overlay.qcow2"
PIDFILE="/tmp/test-runner-vm.pid"

qemu-img create -f qcow2 -b "$(realpath "$QCOW2")" -F qcow2 "$OVERLAY"

ACCEL="kvm"
ACCEL_FLAGS="-enable-kvm -cpu host"
if [ ! -w /dev/kvm ]; then
  echo "KVM not available, falling back to TCG (slower)"
  ACCEL="tcg"
  ACCEL_FLAGS=""
fi

qemu-system-x86_64 \
  ${ACCEL_FLAGS} \
  -m 4096 \
  -smp 4 \
  -drive file="$OVERLAY",if=virtio,format=qcow2 \
  -netdev user,id=net0,hostfwd=tcp::"${SSH_PORT}"-:22 \
  -device virtio-net-pci,netdev=net0 \
  -display none \
  -serial null \
  -daemonize \
  -pidfile "$PIDFILE"

echo "VM started (PID: $(cat "$PIDFILE"), SSH port: ${SSH_PORT})"
