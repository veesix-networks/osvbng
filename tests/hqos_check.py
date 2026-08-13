#!/usr/bin/env python3
# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later
#
# HQoS assertions for the hqos-svlan suites, run on the robot host against a
# deployed lab. Everything is read from the QoS plugin's own CLIs, so the
# checks are identical for IPoE and PPPoE sessions: a subscriber is
# identified by the S-VLAN aggregate it is attached under and its scheduler
# rate, both of which the suites' rate plan makes meaningful.
#
#   hqos_check.py attach  <bng-container>
#       Every S-VLAN aggregate has exactly the members the rate plan calls
#       for, nothing is attached straight to the port, and no scheduler
#       exists that no aggregate claims. Run after churn as well as before:
#       a teardown that half-detaches a scheduler shows up here.
#
#   hqos_check.py measure <bng-container> <seconds>
#       Two samples of the dequeue-side counters <seconds> apart, computes
#       each subscriber's and each S-VLAN's share of the port, and checks
#       them against the expectations below. Shares come from the counters
#       the plugin charges only for packets that actually left: scheduler
#       dequeued bytes and aggregate shaped bytes.
#
#   hqos_check.py drained <bng-container>
#       With every session torn down: no schedulers remain, and every tier
#       has released its weight, its child count and its buffer charge.
#       This is the leak check - weight accounting and buffer charge are
#       both unwound on teardown paths that only session churn exercises.

import re
import subprocess
import sys
import time

# Must match the suites' config: per S-VLAN, its share of the port and the
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


def aggregates(container):
    """Parse `show cake aggregate` into {'port'|<svlan>: {...}}.

    Each tier carries its own counters and the schedulers attached directly
    to it. Sections are sequential in the CLI output - the port and its
    direct members first, then each S-VLAN with its own - so the current
    section header decides who a member line belongs to.
    """
    out, cur = {}, None
    for line in dock(container, *VPPCTL, "show", "cake", "aggregate").splitlines():
        m = re.match(r"^ {2}(\S+): rate \d+ B/s \((\d+) kbps\)", line)
        if m:
            cur = "port"
            out[cur] = {"name": m.group(1), "kbps": int(m.group(2)), "members": {}}
            continue

        m = re.match(r"^ {4}svlan (\d+)-\d+: rate \d+ B/s \((\d+) kbps\)", line)
        if m:
            cur = m.group(1)
            out[cur] = {"name": f"svlan {m.group(1)}", "kbps": int(m.group(2)),
                        "members": {}}
            continue

        if cur is None:
            continue

        m = re.search(r"active (\d+) children, W (\d+)", line)
        if m:
            out[cur]["active_children"] = int(m.group(1))
            out[cur]["active_weight"] = int(m.group(2))
            continue

        m = re.search(r"buffer (\d+)/\d+ bytes, shaped \d+ pkts (\d+) bytes", line)
        if m:
            out[cur]["buffer"] = int(m.group(1))
            out[cur]["shaped"] = int(m.group(2))
            continue

        m = re.match(r"^\s+(\S+): weight (\d+) \(x\d+\), share [\d.]+%, (\w+),", line)
        if m:
            out[cur]["members"][m.group(1)] = {"weight": int(m.group(2)),
                                               "active": m.group(3) == "active"}
    return out


def schedulers(container):
    """{iface-name: {'kbps': rate, 'deq': dequeued bytes}}."""
    out, name = {}, None
    for line in dock(container, *VPPCTL, "show", "cake", "scheduler").splitlines():
        m = re.match(r"^ {2}(\S+): rate \d+ B/s \((\d+) kbps\)", line)
        if m:
            name = m.group(1)
            out[name] = {"kbps": int(m.group(2)), "deq": 0}
            continue
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
    aggs = aggregates(container)
    sched = schedulers(container)
    failures = 0

    for svlan, plan in sorted(EXPECTED.items()):
        members = aggs.get(svlan, {}).get("members", {})
        rates = sorted(sched.get(n, {}).get("kbps", -1) for n in members)
        want = sorted(r for r, _ in plan["members"])
        detail = ", ".join(f"{n}@{sched.get(n, {}).get('kbps', '?')}k" for n in members)
        print(f"  svlan {svlan}: members [{detail}]")
        if rates != want:
            print(f"    FAIL: member rates {rates}, expected {want}")
            failures += 1

    # Every subscriber belongs under an S-VLAN in these topologies, so a
    # scheduler on the port means attachment resolved to the wrong tier.
    stray = list(aggs.get("port", {}).get("members", {}))
    if stray:
        print(f"  FAIL: {len(stray)} scheduler(s) attached straight to the port: {stray}")
        failures += 1

    # A scheduler no aggregate claims is one that outlived its attachment.
    claimed = {n for a in aggs.values() for n in a["members"]}
    orphans = sorted(set(sched) - claimed)
    if orphans:
        print(f"  FAIL: {len(orphans)} scheduler(s) attached to no aggregate: {orphans}")
        failures += 1

    return failures


def cmd_measure(container, seconds):
    agg0, sched0 = aggregates(container), schedulers(container)
    time.sleep(seconds)
    agg1, sched1 = aggregates(container), schedulers(container)

    port_delta = agg1.get("port", {}).get("shaped", 0) - agg0.get("port", {}).get("shaped", 0)
    print(f"port shaped: {port_delta} bytes over {seconds}s "
          f"({port_delta * 8 / seconds / 1000:.0f} kbps)")
    if port_delta < MIN_PORT_BYTES_PER_SEC * seconds:
        print("FAIL: port not saturated - streams are not flowing, shares are meaningless")
        return 1

    failures = 0
    print("s-vlan split of the port:")
    for svlan, plan in sorted(EXPECTED.items()):
        delta = agg1.get(svlan, {}).get("shaped", 0) - agg0.get(svlan, {}).get("shaped", 0)
        failures += not check(svlan, 100.0 * delta / port_delta,
                              plan["share"], SVLAN_TOLERANCE_PTS)

    total = sum(s1["deq"] - sched0.get(n, {}).get("deq", 0) for n, s1 in sched1.items())
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
            for n in agg1.get(svlan, {}).get("members", {}) if n in sched1
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


def cmd_drained(container):
    """With no sessions left, nothing may still be held anywhere."""
    failures = 0

    sched = schedulers(container)
    if sched:
        print(f"  FAIL: {len(sched)} scheduler(s) survived teardown: {sorted(sched)}")
        failures += 1
    else:
        print("  schedulers: none left")

    for key, agg in sorted(aggregates(container).items()):
        name = agg["name"]
        problems = []
        if agg["members"]:
            problems.append(f"{len(agg['members'])} member(s) still listed")
        if agg.get("active_children"):
            problems.append(f"active_children {agg['active_children']}")
        if agg.get("active_weight"):
            problems.append(f"active_weight {agg['active_weight']}")
        if agg.get("buffer"):
            problems.append(f"buffer {agg['buffer']} bytes")
        if problems:
            print(f"  FAIL: {name} did not drain: {', '.join(problems)}")
            failures += 1
        else:
            print(f"  {name}: drained (W 0, 0 children, buffer 0)")

    return failures


def main():
    cmd, container = sys.argv[1], sys.argv[2]
    if cmd == "attach":
        rc = cmd_attach(container)
    elif cmd == "measure":
        rc = cmd_measure(container, int(sys.argv[3]))
    elif cmd == "drained":
        rc = cmd_drained(container)
    else:
        print(f"unknown command {cmd}")
        rc = 2
    sys.exit(1 if rc else 0)


if __name__ == "__main__":
    main()
