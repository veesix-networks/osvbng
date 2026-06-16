#!/usr/bin/env python3
# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

import logging
import math
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
vrnetlab.HOST_FWDS.append(("tcp", 8443, 8443))
vrnetlab.HOST_FWDS.append(("udp", 3799, 3799))
# HA inter-BNG gRPC channel; without this the standby peer can't be
# reached over the QEMU-wrapped mgmt iface.
vrnetlab.HOST_FWDS.append(("tcp", 50051, 50051))


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

        # Create tap devices up-front so QEMU can attach to them at
        # start time, regardless of whether containerlab has wired the
        # external veths in yet. The veth <-> tap tc-mirror bridges are
        # set up lazily in bootstrap_spin once the veths appear.
        #
        # 1500 matches osvbngd's per-interface default (interfaces.go's
        # DefaultMTU) and what containerlab puts on the wrapper veths
        # via the test topology's per-link `mtu` setting. Keeping the
        # tap, the virtio NIC, and the guest kernel interface at the
        # same value avoids VPP af_packet quietly dropping VLAN-tagged
        # frames that exceed VPP's per-interface MTU.
        self._data_mtu = 1500
        for i in range(1, self.num_nics + 1):
            tap = f"tap{i}"
            subprocess.run(["ip", "tuntap", "add", "mode", "tap", tap],
                           check=False)
            subprocess.run(["ip", "link", "set", tap, "mtu",
                            str(self._data_mtu)], check=False)
            subprocess.run(["ip", "link", "set", tap, "up"], check=False)
        self._bridged_nics = set()

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

        # Anything the operator bind-mounted under /etc/osvbng/ needs
        # to land inside the guest, not just at the wrapper container's
        # filesystem. The big example is mTLS test certs; newer osvbngd
        # validates cert files at config-load time, so a missing
        # cert_file path crash-loops the daemon. Pull every regular
        # file under /etc/osvbng/ into write_files, binary-encoded so
        # certs/keys survive the YAML round-trip intact.
        import base64
        write_files = []
        for root, _, files in os.walk("/etc/osvbng"):
            for name in files:
                src = os.path.join(root, name)
                try:
                    data = open(src, "rb").read()
                except OSError as e:
                    self.logger.warning("skipping %s: %s", src, e)
                    continue
                write_files.append((src, data))
        if write_files:
            lines.append("write_files:")
        for src, data in write_files:
            lines.append(f"  - path: {src}")
            lines.append("    owner: root:root")
            lines.append("    permissions: '0644'")
            lines.append("    encoding: b64")
            lines.append("    content: " + base64.b64encode(data).decode("ascii"))
        if not os.path.exists(cfg_path):
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

    def gen_nics(self):
        """Override the base socket-listen netdev with tap netdevs so
        each data NIC binds to a host tap that's bridged (via
        tc-mirror) to the container's matching ethN veth from
        containerlab. Without this, the BNG's data path is unreachable
        from the rest of the topology (subscriber container, core
        router) because base vrnetlab assumes the peer is another QEMU
        instance on a socket pair."""
        res = []
        for i in range(1, self.num_nics + 1):
            pci_bus = math.floor(i / self.nics_per_pci_bus) + 1
            addr = (i % self.nics_per_pci_bus) + 1
            res.extend([
                "-device",
                f"{self.nic_type},netdev=p{i:02d},mac={vrnetlab.gen_mac(i)},"
                f"bus=pci.{pci_bus},addr=0x{addr:x},"
                f"host_mtu={self._data_mtu}",
                "-netdev",
                f"tap,id=p{i:02d},ifname=tap{i},script=no,downscript=no",
            ])
        return res

    def _bridge_pending_veths(self):
        """For every container eth that's appeared and not yet bridged,
        wire tc-mirror in both directions between ethN and tapN. Called
        on every bootstrap tick until all data NICs are stitched."""
        for i in range(1, self.num_nics + 1):
            if i in self._bridged_nics:
                continue
            eth = f"eth{i}"
            if subprocess.run(["ip", "link", "show", eth],
                              capture_output=True).returncode != 0:
                continue
            tap = f"tap{i}"
            subprocess.run(["ip", "link", "set", eth, "up"], check=False)
            for src, dst in ((eth, tap), (tap, eth)):
                subprocess.run(["tc", "qdisc", "add", "dev", src, "clsact"],
                               check=False, stderr=subprocess.DEVNULL)
                subprocess.run([
                    "tc", "filter", "add", "dev", src, "ingress",
                    "matchall", "action", "mirred", "egress",
                    "redirect", "dev", dst,
                ], check=False, stderr=subprocess.DEVNULL)
            self._bridged_nics.add(i)
            self.logger.info("bridged %s <-> %s via tc-mirror", eth, tap)

    def bootstrap_spin(self):
        """No console expect/login dance — cloud-init does it all. Just
        wait until ssh into the VM works and osvbngd reports ready."""
        self._bridge_pending_veths()
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
            # The QEMU build has DPDK disabled and VPP attaches to data
            # NICs via af_packet, which depends on the kernel forwarding
            # frames to the raw socket. Without promisc, broadcast DHCP
            # discovers and OSPF multicast hellos never reach VPP, so
            # subscriber sessions and routing adjacencies don't form.
            for i in range(1, self.num_nics + 1):
                subprocess.run(
                    ["ssh", "-i", self.ssh_key_path,
                     "-o", "StrictHostKeyChecking=no",
                     "-o", "UserKnownHostsFile=/dev/null",
                     "-o", "LogLevel=ERROR",
                     "-p", "2022", "root@127.0.0.1",
                     f"ip link set eth{i} mtu {self._data_mtu} promisc on"],
                    check=False, capture_output=True,
                )
            # Surface a stable marker on container stdout so
            # `docker logs <wrapper> | grep "osvbng started successfully"`
            # works the same way it does for the Docker-native build.
            print("osvbng started successfully", flush=True)
            # Tail the guest's osvbngd journal to the wrapper's stdout so
            # Robot keywords that grep `docker logs` for runtime markers
            # (`cgnat reconcile: soft-update pool`, etc.) see the same
            # stream they'd see from a Docker-native osvbngd.
            subprocess.Popen(
                ["ssh", "-i", self.ssh_key_path,
                 "-o", "StrictHostKeyChecking=no",
                 "-o", "UserKnownHostsFile=/dev/null",
                 "-o", "LogLevel=ERROR",
                 "-o", "ServerAliveInterval=30",
                 "-p", "2022", "root@127.0.0.1",
                 "journalctl -u osvbng -f -n 0 -o cat"],
                stdout=sys.stdout, stderr=subprocess.STDOUT,
            )
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
