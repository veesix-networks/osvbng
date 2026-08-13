#!/usr/bin/env python3
# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later
#
# HQoS assertions for suite 51, run on the robot host against a deployed lab.
#
#   hqos_check.py attach  <bng-container>
#       Every S-VLAN aggregate has exactly the schedulers that belong to it.
#
#   hqos_check.py measure <bng-container> <seconds>
#       Two samples of the dequeue-side counters <seconds> apart, computes each
#       subscriber's and each S-VLAN's share of the port, and checks them
#       against the expectations below. Shares come from the same counters the
#       plugin's own fairness rig uses: scheduler dequeued bytes and aggregate
#       shaped bytes, both charged only for packets that actually left.

import json
import re
import subprocess
import sys
import time
import urllib.request

# Must match config/bng1/osvbng.yaml. Shares are percent of the port.
EXPECTED_SVLAN = {"100": 50.0, "200": 25.0, "300": 25.0}
EXPECTED_SUBS = {
    "100.10": 10.0,
    "100.11": 40.0,
    "200.10": 12.5,
    "200.11": 12.5,
    "300.10": 12.5,
    "300.11": 12.5,
}
SVLAN_TOLERANCE_PTS = 5.0
SUB_TOLERANCE_PTS = 4.0
# A run that moved almost nothing must not report shares; ~8 Mbit/s for the
# window is the port rate, so a third of that means real saturation.
MIN_PORT_BYTES_PER_SEC = 8000 * 1000 / 8 / 3

VPPCTL = ["vppctl", "-s", "/run/osvbng/cli.sock"]


def dock(container, *cmd):
    return subprocess.run(
        ["sudo", "docker", "exec", container, *cmd],
        capture_output=True, text=True, check=True,
    ).stdout


def bng_ip(container):
    return subprocess.run(
        ["sudo", "docker", "inspect", "-f",
         "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", container],
        capture_output=True, text=True, check=True,
    ).stdout.strip()


def api(container, path):
    with urllib.request.urlopen(f"http://{bng_ip(container)}:8080{path}", timeout=10) as r:
        return json.load(r)


def session_vlans_by_ifindex(container):
    out = {}
    for s in api(container, "/api/show/subscriber/sessions").get("data") or []:
        out[s["IfIndex"]] = f'{s["OuterVLAN"]}.{s["InnerVLAN"]}'
    return out


def interface_indexes(container):
    out = {}
    for line in dock(container, *VPPCTL, "show", "interface").splitlines():
        m = re.match(r"^(\S+)\s+(\d+)\s+", line)
        if m and m.group(1) != "Name":
            out[m.group(1)] = int(m.group(2))
    return out


def aggregate_shaped_bytes(container):
    """{'port': bytes, '100': bytes, ...} from `show cake aggregate`."""
    out, key = {}, None
    for line in dock(container, *VPPCTL, "show", "cake", "aggregate").splitlines():
        m = re.match(r"^\s{2}(\S+): rate \d+ B/s", line)
        if m:
            key = "port"
        m = re.match(r"^\s+svlan (\d+)-\d+: rate", line)
        if m:
            key = m.group(1)
        m = re.search(r"shaped \d+ pkts (\d+) bytes", line)
        if m and key is not None:
            out[key] = int(m.group(1))
            key = None
    return out


def scheduler_dequeued_bytes(container):
    """{iface-name: dequeued bytes} from `show cake scheduler`."""
    out, name = {}, None
    for line in dock(container, *VPPCTL, "show", "cake", "scheduler").splitlines():
        m = re.match(r"^\s{2}(\S+): rate \d+ B/s", line)
        if m:
            name = m.group(1)
        m = re.search(r"dequeued: \d+ pkts (\d+) bytes", line)
        if m and name is not None:
            out[name] = int(m.group(1))
            name = None
    return out


def svlan_members(container):
    """{svlan: [scheduler names]} from the aggregate tree."""
    out, key = {}, None
    for line in dock(container, *VPPCTL, "show", "cake", "aggregate").splitlines():
        m = re.match(r"^\s+svlan (\d+)-\d+: rate", line)
        if m:
            key = m.group(1)
            out[key] = []
            continue
        m = re.match(r"^\s+(\S+): weight \d+ \(x\d+\), share", line)
        if m and key is not None:
            out[key].append(m.group(1))
    return out


def check(label, got, want, tol):
    err = got - want
    ok = abs(err) <= tol
    print(f"  {label:<10} got {got:6.2f}%  want {want:6.2f}%  err {err:+6.2f} pts"
          f"  [{'ok' if ok else 'FAIL'}]")
    return ok


def cmd_attach(container):
    ifidx = interface_indexes(container)
    vlans = session_vlans_by_ifindex(container)
    members = svlan_members(container)

    failures = 0
    for svlan in EXPECTED_SVLAN:
        names = members.get(svlan, [])
        got = sorted(vlans.get(ifidx.get(n, -1), f"?{n}") for n in names)
        want = sorted(k for k in EXPECTED_SUBS if k.startswith(f"{svlan}."))
        print(f"  svlan {svlan}: members {got}")
        if got != want:
            print(f"    FAIL: expected {want}")
            failures += 1
    return failures


def cmd_measure(container, seconds):
    ifidx = interface_indexes(container)
    vlans = session_vlans_by_ifindex(container)
    name_to_vlan = {n: vlans[i] for n, i in ifidx.items() if i in vlans}

    agg0, sched0 = aggregate_shaped_bytes(container), scheduler_dequeued_bytes(container)
    time.sleep(seconds)
    agg1, sched1 = aggregate_shaped_bytes(container), scheduler_dequeued_bytes(container)

    port_delta = agg1.get("port", 0) - agg0.get("port", 0)
    print(f"port shaped: {port_delta} bytes over {seconds}s "
          f"({port_delta * 8 / seconds / 1000:.0f} kbps)")
    if port_delta < MIN_PORT_BYTES_PER_SEC * seconds:
        print("FAIL: port not saturated - streams are not flowing, shares are meaningless")
        return 1

    failures = 0
    print("s-vlan split of the port:")
    for svlan, want in sorted(EXPECTED_SVLAN.items()):
        delta = agg1.get(svlan, 0) - agg0.get(svlan, 0)
        failures += not check(svlan, 100.0 * delta / port_delta, want, SVLAN_TOLERANCE_PTS)

    sub_delta = {}
    for name, b1 in sched1.items():
        key = name_to_vlan.get(name)
        if key:
            sub_delta[key] = b1 - sched0.get(name, 0)
    total = sum(sub_delta.values())
    if total <= 0:
        print("FAIL: no scheduler dequeued anything")
        return 1

    print("subscriber split of the port:")
    for key, want in sorted(EXPECTED_SUBS.items()):
        got = 100.0 * sub_delta.get(key, 0) / total
        failures += not check(key, got, want, SUB_TOLERANCE_PTS)

    return failures


def main():
    cmd, container = sys.argv[1], sys.argv[2]
    if cmd == "attach":
        rc = cmd_attach(container)
    elif cmd == "measure":
        rc = cmd_measure(container, int(sys.argv[3]))
    else:
        print(f"unknown command {cmd}")
        rc = 2
    sys.exit(1 if rc else 0)


if __name__ == "__main__":
    main()
