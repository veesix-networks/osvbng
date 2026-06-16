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
networking setup is untouched.

For `pkill` of osvbngd or vpp_main: rsync /etc/osvbng/ into the guest
before forwarding the kill. Docker-mode tests edit the bind-mounted
config in place and expect the next osvbngd restart to pick the diff up;
in QEMU mode the config seed only runs at first boot, so without a
pre-restart sync the guest's `osvbngd` re-reads the same file and the
soft-update/reconcile log markers the suite greps for never get emitted."""

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

ssh_args = [
    "-i", "/root/.ssh/id_ed25519",
    "-o", "StrictHostKeyChecking=no",
    "-o", "UserKnownHostsFile=/dev/null",
    "-o", "LogLevel=ERROR",
]
ssh = ["ssh", *ssh_args, "-p", "2022", "root@127.0.0.1"]

if cmd == "pkill" and any(a in ("osvbngd", "vpp_main") for a in sys.argv[1:]):
    subprocess.run(
        ["rsync", "-aq", "--no-owner", "--no-group",
         "-e", " ".join(["ssh", *ssh_args, "-p", "2022"]),
         "/etc/osvbng/", "root@127.0.0.1:/etc/osvbng/"],
        check=False,
    )

remote = " ".join(shlex.quote(a) for a in [cmd, *sys.argv[1:]])
sys.exit(subprocess.call(ssh + [remote]))
