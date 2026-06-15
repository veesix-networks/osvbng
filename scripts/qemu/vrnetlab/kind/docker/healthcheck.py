#!/usr/bin/env python3
# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

"""Docker HEALTHCHECK: green when ssh into the wrapped VM works and
osvbngd reports state=ready."""

import subprocess
import sys

KEY = "/root/.ssh/id_ed25519"

res = subprocess.run(
    [
        "ssh", "-i", KEY,
        "-o", "StrictHostKeyChecking=no",
        "-o", "UserKnownHostsFile=/dev/null",
        "-o", "LogLevel=ERROR",
        "-o", "ConnectTimeout=5",
        "-p", "2022", "root@127.0.0.1",
        'test -f /run/osvbng/state && grep -q \'"state":"ready"\' /run/osvbng/state',
    ],
    capture_output=True,
    timeout=10,
)
sys.exit(res.returncode)
