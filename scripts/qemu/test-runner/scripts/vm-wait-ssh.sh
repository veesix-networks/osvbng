#!/bin/bash
# Copyright 2025 Veesix Networks Ltd
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

set -euo pipefail

SSH_PORT="${1:-2222}"
TIMEOUT="${2:-120}"

SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ConnectTimeout=5"

elapsed=0
while [ "$elapsed" -lt "$TIMEOUT" ]; do
  if sshpass -p testrunner ssh ${SSH_OPTS} -p "${SSH_PORT}" root@localhost true 2>/dev/null; then
    echo "SSH ready after ${elapsed}s"
    exit 0
  fi
  sleep 2
  elapsed=$((elapsed + 2))
done

echo "ERROR: SSH not ready after ${TIMEOUT}s"
exit 1
