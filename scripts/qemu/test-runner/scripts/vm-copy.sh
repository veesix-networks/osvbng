#!/bin/bash
# Copyright 2025 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

set -euo pipefail

DIRECTION="${1:?Usage: vm-copy.sh <to|from> <src> <dst> [ssh-port]}"
SRC="${2:?Missing source}"
DST="${3:?Missing destination}"
SSH_PORT="${4:-2222}"

SCP_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR"

case "${DIRECTION}" in
  to)
    sshpass -p testrunner scp ${SCP_OPTS} -P "${SSH_PORT}" -r "${SRC}" "root@localhost:${DST}"
    ;;
  from)
    sshpass -p testrunner scp ${SCP_OPTS} -P "${SSH_PORT}" -r "root@localhost:${SRC}" "${DST}"
    ;;
  *)
    echo "ERROR: direction must be 'to' or 'from'"
    exit 1
    ;;
esac
