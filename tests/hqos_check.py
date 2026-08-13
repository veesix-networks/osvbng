#!/usr/bin/env python3
# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later
#
# HQoS assertions for the hqos-svlan suites, run on the robot host against a
# deployed lab. Everything is read from the QoS plugin's own CLIs, so the
# checks are identical for IPoE and PPPoE sessions: a subscriber is
# identified by the S-VLAN aggregate it is attached under and its scheduler
# rate, both of which the suite's rate plan makes meaningful.
#
#   hqos_check.py attach  <bng-container>
#       Every S-VLAN aggregate has exactly the members the rate plan calls
#       for: two schedulers, at the rates configured for that S-VLAN.
#
#   hqos_check.py measure <bng-container> <seconds>
#       Two samples of the dequeue-side counters <seconds> apart, computes
#       each subscriber's and each S-VLAN's share of the port, and checks
#       them against the expectations below. Shares come from the counters
#       the plugin charges only for packets that actually left: scheduler
#       dequeued bytes and aggregate shaped bytes.

import re
import subprocess
import sys
import time

# Must match the suite's config: per S-VLAN, its share of the port and the
# per-subscriber (scheduler-rate -> share-of-port) plan. Duplicate rates are
# listed once per member.
EXPECTED = {
    "100": {"share": 50.0, "members": [(2000, 10.0), (8000, 40.0)]},
    "200": {"share": 25.0, "members": [(4000, 12.5), (4000, 12.5)]},
    "300": {"share": 25.0, "members": [(4000, 12.5), (4000, 12.5)]},
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


def schedulers(container):
    """{iface-name: {'kbps': rate, 'deq': dequeued bytes}}."""
    out, name = {}, None
    for line in dock(container, *VPPCTL, "show", "cake", "scheduler").splitlines():
        m = re.match(r"^\s{2}(\S+): rate \d+ B/s \((\d+) kbps\)", line)
        if m:
            name = m.group(1)
            out[name] = {"kbps": int(m.group(2)), "deq": 0}
        m = re.search(r"dequeued: \d+ pkts (\d+) bytes", line)
        if m and name is not None:
            out[name]["deq"] = int(m.group(1))
            name = None
    return out


def check(label, got, want, tol):
    err = got - want
    ok = abs(err) <= tol
    print(f"  {label:<12} got {got:6.2f}%  want {want:6.2f}%  err {err:+6.2f} pts"
          f"  [{'ok' if ok else 'FAIL'}]")
    return ok


def cmd_attach(container):
    members = svlan_members(container)
    sched = schedulers(container)

    failures = 0
    for svlan, plan in sorted(EXPECTED.items()):
        names = members.get(svlan, [])
        rates = sorted(sched.get(n, {}).get("kbps", -1) for n in names)
        want = sorted(r for r, _ in plan["members"])
        detail = ", ".join(f"{n}@{sched.get(n, {}).get('kbps', '?')}k" for n in names)
        print(f"  svlan {svlan}: members [{detail}]")
        if rates != want:
            print(f"    FAIL: member rates {rates}, expected {want}")
            failures += 1
    return failures


def cmd_measure(container, seconds):
    members = svlan_members(container)

    agg0, sched0 = aggregate_shaped_bytes(container), schedulers(container)
    time.sleep(seconds)
    agg1, sched1 = aggregate_shaped_bytes(container), schedulers(container)

    port_delta = agg1.get("port", 0) - agg0.get("port", 0)
    print(f"port shaped: {port_delta} bytes over {seconds}s "
          f"({port_delta * 8 / seconds / 1000:.0f} kbps)")
    if port_delta < MIN_PORT_BYTES_PER_SEC * seconds:
        print("FAIL: port not saturated - streams are not flowing, shares are meaningless")
        return 1

    failures = 0
    print("s-vlan split of the port:")
    for svlan, plan in sorted(EXPECTED.items()):
        delta = agg1.get(svlan, 0) - agg0.get(svlan, 0)
        failures += not check(svlan, 100.0 * delta / port_delta,
                              plan["share"], SVLAN_TOLERANCE_PTS)

    total = sum(s1["deq"] - sched0.get(n, {}).get("deq", 0)
                for n, s1 in sched1.items())
    if total <= 0:
        print("FAIL: no scheduler dequeued anything")
        return 1

    print("subscriber split of the port:")
    for svlan, plan in sorted(EXPECTED.items()):
        # Pair members with expectations by scheduler rate; members sharing a
        # rate share an expectation, so pairing within a rate is arbitrary.
        got = sorted(
            (sched1[n]["kbps"],
             100.0 * (sched1[n]["deq"] - sched0.get(n, {}).get("deq", 0)) / total)
            for n in members.get(svlan, []) if n in sched1
        )
        want = sorted(plan["members"])
        if len(got) != len(want):
            print(f"  svlan {svlan}: FAIL: {len(got)} measurable members, expected {len(want)}")
            failures += 1
            continue
        for (kbps, share), (want_kbps, want_share) in zip(got, want):
            label = f"{svlan} @{kbps}k"
            if kbps != want_kbps:
                print(f"  {label:<12} FAIL: rate {kbps}, expected {want_kbps}")
                failures += 1
                continue
            failures += not check(label, share, want_share, SUB_TOLERANCE_PTS)

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
