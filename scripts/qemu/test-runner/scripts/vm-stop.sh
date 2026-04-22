#!/bin/bash
# Copyright 2025 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

set -euo pipefail

SSH_PORT="${1:-2222}"
PIDFILE="/tmp/test-runner-vm.pid"
OVERLAY="/tmp/test-runner-overlay.qcow2"

SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ConnectTimeout=5"

sshpass -p testrunner ssh ${SSH_OPTS} -p "${SSH_PORT}" root@localhost "shutdown -h now" 2>/dev/null || true

if [ -f "$PIDFILE" ]; then
  PID=$(cat "$PIDFILE")
  for _ in $(seq 1 15); do
    if ! kill -0 "$PID" 2>/dev/null; then
      break
    fi
    sleep 1
  done
  kill -9 "$PID" 2>/dev/null || true
  rm -f "$PIDFILE"
fi

rm -f "$OVERLAY"
echo "VM stopped"
