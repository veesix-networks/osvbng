#!/usr/bin/env python3
# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

"""Forward this invocation to the same-named binary inside the QEMU VM
via ssh. Symlinked as vppctl, vtysh, ip so that
`docker exec <wrapper> vppctl ...` runs the real command in the BNG VM
with no caller changes.

For `ip` specifically: only `ip netns exec <ns> <cmd> ...` is forwarded
(the `dataplane` netns lives inside the VM, not the wrapper). All other
`ip` invocations exec the real /usr/sbin/ip so the wrapper's own
networking setup is untouched."""

import os
import shlex
import subprocess
import sys

cmd = os.path.basename(sys.argv[0])

if cmd == "ip":
    if not (len(sys.argv) >= 5
            and sys.argv[1] == "netns"
            and sys.argv[2] == "exec"):
        os.execv("/usr/sbin/ip", sys.argv)

ssh = [
    "ssh", "-i", "/root/.ssh/id_ed25519",
    "-o", "StrictHostKeyChecking=no",
    "-o", "UserKnownHostsFile=/dev/null",
    "-o", "LogLevel=ERROR",
    "-p", "2022", "root@127.0.0.1",
]
remote = " ".join(shlex.quote(a) for a in [cmd, *sys.argv[1:]])
sys.exit(subprocess.call(ssh + [remote]))
