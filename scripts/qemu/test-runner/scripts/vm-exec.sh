#!/bin/bash
# Copyright 2025 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

set -euo pipefail

COMMAND="${1:?Usage: vm-exec.sh <command> [ssh-port]}"
SSH_PORT="${2:-2222}"

SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR"

sshpass -p testrunner ssh ${SSH_OPTS} -p "${SSH_PORT}" root@localhost "${COMMAND}"
