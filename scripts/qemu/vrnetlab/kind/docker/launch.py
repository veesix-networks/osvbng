#!/usr/bin/env python3
# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

import logging
import os
import signal
import subprocess
import sys

import vrnetlab


def handle_SIGTERM(_signal, _frame):
    sys.exit(0)


signal.signal(signal.SIGINT, handle_SIGTERM)
signal.signal(signal.SIGTERM, handle_SIGTERM)

logging.basicConfig(stream=sys.stdout, level=logging.DEBUG)

# Expose the northbound API and prometheus exporter to the wrapper
# container's eth0 alongside the default ssh forward.
vrnetlab.HOST_FWDS.append(("tcp", 8080, 8080))
vrnetlab.HOST_FWDS.append(("tcp", 9090, 9090))


class Osvbng_vm(vrnetlab.VM):
    """osvbng has no console-driven bootstrap — cloud-init in the qcow2
    consumes a NoCloud seed ISO at first boot to inject the operator's
    osvbng.yaml and the ssh authorized_key, and systemd brings vpp +
    osvbngd up itself. So launch.py only has to: render the seed,
    attach it as a second drive, then wait until ssh works."""

    def __init__(self, hostname: str, num_nics: int):
        super().__init__("root", "", disk_image="/osvbng.qcow2", ram=8192)
        self.hostname = hostname
        self.num_nics = num_nics
        self.nic_type = "virtio-net-pci"

        # vrnetlab's default qemu_args use the generic qemu CPU model,
        # which doesn't expose SSE4.2. VPP refuses to start without it
        # ("ERROR: This binary requires CPU with SSE4.2 extensions").
        # Pass through host CPU features so the guest sees whatever the
        # runner has.
        self.qemu_args.extend(["-cpu", "host"])

        # Generate an ssh keypair the launcher (and healthcheck) use to
        # talk to the VM. cloud-init writes the pub key to
        # /root/.ssh/authorized_keys.
        self.ssh_key_path = "/root/.ssh/id_ed25519"
        os.makedirs("/root/.ssh", exist_ok=True)
        subprocess.run(
            ["ssh-keygen", "-t", "ed25519", "-N", "", "-f", self.ssh_key_path, "-q"],
            check=True,
        )
        pub = open(self.ssh_key_path + ".pub").read().strip()

        # Render the seed ISO and attach as a second drive.
        seed_iso = self._build_seed_iso(pub)
        self.qemu_args.extend(
            ["-drive", f"if=virtio,format=raw,readonly=on,file={seed_iso}"]
        )

    def _build_seed_iso(self, ssh_pub_key: str) -> str:
        cfg_path = "/etc/osvbng/osvbng.yaml"
        lines = [
            "#cloud-config",
            "ssh_authorized_keys:",
            f"  - {ssh_pub_key}",
            "disable_root: false",
            "ssh_pwauth: false",
            "package_update: false",
        ]
        if os.path.exists(cfg_path):
            lines.append("write_files:")
            lines.append("  - path: /etc/osvbng/osvbng.yaml")
            lines.append("    owner: root:root")
            lines.append("    permissions: '0644'")
            lines.append("    content: |")
            for line in open(cfg_path).read().splitlines():
                lines.append("      " + line)
        else:
            self.logger.warning(
                "%s not present; VM will boot with osvbngd default config", cfg_path
            )

        os.makedirs("/tmp/seed", exist_ok=True)
        open("/tmp/seed/user-data", "w").write("\n".join(lines) + "\n")
        open("/tmp/seed/meta-data", "w").write(
            f"instance-id: {self.hostname}\nlocal-hostname: {self.hostname}\n"
        )
        seed = "/tmp/osvbng-seed.iso"
        subprocess.run(
            [
                "xorrisofs", "-quiet", "-output", seed,
                "-volid", "CIDATA", "-joliet", "-rock",
                "/tmp/seed/user-data", "/tmp/seed/meta-data",
            ],
            check=True,
        )
        return seed

    def bootstrap_spin(self):
        """No console expect/login dance — cloud-init does it all. Just
        wait until ssh into the VM works and osvbngd reports ready."""
        if self.spins > 240:
            self.logger.warning("osvbng not ready after %ds, restarting VM", self.spins)
            self.stop()
            self.start()
            self.spins = 0
            return

        res = subprocess.run(
            [
                "ssh", "-i", self.ssh_key_path,
                "-o", "StrictHostKeyChecking=no",
                "-o", "UserKnownHostsFile=/dev/null",
                "-o", "LogLevel=ERROR",
                "-o", "ConnectTimeout=3",
                "-p", "2022", "root@127.0.0.1",
                'test -f /run/osvbng/state && grep -q \'"state":"ready"\' /run/osvbng/state',
            ],
            capture_output=True,
            timeout=10,
        )
        if res.returncode == 0:
            self.running = True
            self.logger.info("osvbng reached state=ready after %ds", self.spins)
            # Surface a stable marker on container stdout so
            # `docker logs <wrapper> | grep "osvbng started successfully"`
            # works the same way it does for the Docker-native build.
            print("osvbng started successfully", flush=True)
            return

        self.spins += 1


class Osvbng(vrnetlab.VR):
    def __init__(self, hostname: str, num_nics: int):
        super().__init__("root", "")
        self.vms = [Osvbng_vm(hostname, num_nics)]


if __name__ == "__main__":
    hostname = os.getenv("CLAB_NODE_NAME", "osvbng")
    num_nics = int(os.getenv("OSVBNG_NUM_INTERFACES", "3"))
    Osvbng(hostname, num_nics).start()
