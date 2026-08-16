#!/usr/bin/env python3
# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Assertion helpers for the HQoS robot suites (51/52), in the style of
# hqos_check.py: a subcommand, file-path arguments, exit 0/1. Every input
# arrives as a file the suite captured beforehand, so there is no inline
# python and no shell-quoting surface in the .robot files at all.
#
# `selftest` runs every subcommand against the committed fixtures in
# tests/fixtures/qos/ (positive and negative), so a broken assertion fails
# in seconds on a laptop instead of mid-lab-run. Suite 51 runs it as its
# first test case. The CLI fixtures are the same golden files the Go tests
# in cmd/osvbngcli pin whitespace-exactly; assertions here are structural
# (tokens, counts) on purpose.

import json
import re
import sys
from pathlib import Path

UUID_RE = re.compile(r"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")
FIXTURES = Path(__file__).parent / "fixtures" / "qos"


def load_data(path):
    """Read a captured API response, unwrapping the {path, data} envelope."""
    doc = json.loads(Path(path).read_text())
    if isinstance(doc, dict) and "data" in doc:
        return doc["data"]
    return doc


def parse_int_map(spec):
    """'100:6000,200:3000' -> {100: 6000, 200: 3000}"""
    out = {}
    for part in spec.split(","):
        k, v = part.split(":")
        out[int(k)] = int(v)
    return out


def fail(msg):
    print(f"FAIL: {msg}")
    return 1


def ok(msg):
    print(msg)
    return 0


def cmd_aggregates_programmed(path, port_kbps, svlan_spec):
    rows = load_data(path) or []
    ports = [a for a in rows if a["level"] == "port"]
    svlans = {a["svlan_id"]: a["rate_kbps"] for a in rows if a["level"] == "svlan"}
    if len(ports) != 1 or ports[0]["rate_kbps"] != int(port_kbps):
        return fail(f"port aggregates {ports}")
    want = parse_int_map(svlan_spec)
    if svlans != want:
        return fail(f"svlan map {svlans}, want {want}")
    return ok(f"port {port_kbps} kbps, svlans {svlans}")


def cmd_scheduler_rates(path, spec):
    rows = load_data(path) or []
    counts = {}
    for s in rows:
        counts[s["rate_kbps"]] = counts.get(s["rate_kbps"], 0) + 1
    want = parse_int_map(spec)
    if counts != want:
        return fail(f"rate histogram {counts}, want {want}")
    return ok(f"scheduler rates {counts}")


def cmd_aggregate_detail(path, svlan_spec, members):
    d = load_data(path)
    if d["aggregate"]["level"] != "port":
        return fail(f"root is {d['aggregate']['level']}, not port")
    children = {c["aggregate"]["svlan_id"]: c for c in d.get("children") or []}
    want = sorted(int(s) for s in svlan_spec.split(","))
    if sorted(children) != want:
        return fail(f"children {sorted(children)}, want {want}")
    got_members = {sv: len(c.get("schedulers") or []) for sv, c in children.items()}
    if any(n != int(members) for n in got_members.values()):
        return fail(f"member counts {got_members}, want {members} each")
    missing = [s.get("sw_if_index") for c in children.values()
               for s in c["schedulers"] if not s.get("session_id")]
    if missing:
        return fail(f"members without session_id: sw_if_index {missing}")
    return ok(f"hierarchy port -> {want}, {members} members each, all with session ids")


def cmd_session_view(path, access_type=None):
    d = load_data(path)
    if not d.get("session_id"):
        return fail(f"no session_id in view: {d}")
    if access_type and d.get("access_type") != access_type:
        return fail(f"access_type {d.get('access_type')}, want {access_type}")
    sched = d.get("scheduler")
    if not sched or not sched.get("rate_kbps"):
        return fail(f"no scheduler on session {d['session_id']}: {d.get('note')}")
    if not d.get("parent_svlan") or not d.get("parent_port"):
        return fail(f"missing parent tiers for {d['session_id']}")
    return ok(f"{d['session_id']} -> {sched['rate_kbps']} kbps under svlan "
              f"{d['parent_svlan']['svlan_id']}")


def cmd_pick_session_id(path):
    rows = load_data(path) or []
    if not rows:
        print("FAIL: no sessions")
        return 1
    print(rows[0]["SessionID"])
    return 0


def cmd_tin_metrics(path, expect):
    text = Path(path).read_text()
    rows = re.findall(r"osvbng_qos_scheduler_tin_packets\{([^}]*)\}", text)
    if not rows:
        return fail("no tin_packets series scraped")
    labelled = [r for r in rows if re.search(r'tin="\d+"', r)]
    if len(labelled) != len(rows):
        return fail(f"unlabelled tin series: {len(rows) - len(labelled)} of {len(rows)}")
    if len(labelled) != int(expect):
        return fail(f"{len(labelled)} labelled series, want {expect}")
    return ok(f"{len(labelled)} tin series, all labelled")


def cmd_no_sessions(path):
    rows = load_data(path) or []
    if rows:
        return fail(f"{len(rows)} session(s) still present")
    return ok("no sessions remain")


def cmd_cli_scheduler_table(path, sessions):
    text = Path(path).read_text()
    for token in ("SW_IF", "INTERFACE", "SESSION", "RATE", "TX PKTS/BYTES", "BLK D/P"):
        if token not in text:
            return fail(f"header token {token!r} missing:\n{text}")
    uuids = set(UUID_RE.findall(text))
    if len(uuids) != int(sessions):
        return fail(f"{len(uuids)} full-length session uuids, want {sessions}:\n{text}")
    if "{" in text:
        return fail("table contains a JSON blob")
    if "Connected to" in text:
        return fail("output contains the interactive banner")
    return ok(f"compact scheduler table with {len(uuids)} full session uuids")


def cmd_cli_aggregate_tree(path, svlan_spec):
    text = Path(path).read_text()
    for tag in svlan_spec.split(","):
        if not re.search(rf"svlan {tag}\b", text):
            return fail(f"svlan {tag} line missing:\n{text}")
    if not re.search(r"^\S+  port  ", text, re.M):
        return fail(f"port line missing:\n{text}")
    if "shaped" not in text or "pkts" not in text or "buf" not in text:
        return fail(f"counter line tokens missing:\n{text}")
    if "{" in text:
        return fail("tree contains a JSON blob")
    return ok(f"aggregate tree with svlans {svlan_spec}")


def expect(name, rc, want):
    status = "ok" if rc == want else "SELFTEST FAILURE"
    print(f"  [{status}] {name} (rc={rc}, want {want})")
    return 0 if rc == want else 1


def cmd_selftest():
    f = FIXTURES
    failures = 0
    print(f"selftest against {f}")
    failures += expect("aggregates-programmed",
                       cmd_aggregates_programmed(f / "aggregate_api_single_port.json", "8000",
                                                 "100:6000,200:3000,300:3000"), 0)
    # aggregate_api.json spans three ports, so the strict single-port check
    # must reject it.
    failures += expect("aggregates-programmed multi-port",
                       cmd_aggregates_programmed(f / "aggregate_api.json", "8000",
                                                 "100:6000,200:3000,300:3000"), 1)
    failures += expect("scheduler-rates",
                       cmd_scheduler_rates(f / "scheduler_api.json", "2000:1,8000:1,4000:4"), 0)
    failures += expect("scheduler-rates wrong spec",
                       cmd_scheduler_rates(f / "scheduler_api.json", "2000:6"), 1)
    failures += expect("aggregate-detail",
                       cmd_aggregate_detail(f / "aggregate_detail_api.json", "100,200,300", "2"), 0)
    failures += expect("aggregate-detail missing session",
                       cmd_aggregate_detail(f / "aggregate_detail_missing_session.json",
                                            "100,200,300", "2"), 1)
    failures += expect("session-view ipoe",
                       cmd_session_view(f / "session_view_ipoe.json", "ipoe"), 0)
    failures += expect("session-view pppoe",
                       cmd_session_view(f / "session_view_pppoe.json", "pppoe"), 0)
    failures += expect("session-view no scheduler",
                       cmd_session_view(f / "session_view_no_sched.json"), 1)
    failures += expect("pick-session-id",
                       cmd_pick_session_id(f / "sessions_api.json"), 0)
    failures += expect("pick-session-id empty",
                       cmd_pick_session_id(f / "sessions_empty.json"), 1)
    failures += expect("tin-metrics",
                       cmd_tin_metrics(f / "metrics_good.txt", "6"), 0)
    failures += expect("tin-metrics unlabelled",
                       cmd_tin_metrics(f / "metrics_unlabelled.txt", "6"), 1)
    failures += expect("no-sessions",
                       cmd_no_sessions(f / "sessions_empty.json"), 0)
    failures += expect("no-sessions populated",
                       cmd_no_sessions(f / "sessions_api.json"), 1)
    failures += expect("cli-scheduler-table",
                       cmd_cli_scheduler_table(f / "scheduler_cli.txt", "5"), 0)
    failures += expect("cli-scheduler-table wrong input",
                       cmd_cli_scheduler_table(f / "aggregate_cli.txt", "5"), 1)
    failures += expect("cli-aggregate-tree",
                       cmd_cli_aggregate_tree(f / "aggregate_cli.txt", "100,200,300,400-499"), 0)
    failures += expect("cli-aggregate-tree wrong input",
                       cmd_cli_aggregate_tree(f / "scheduler_cli.txt", "100"), 1)
    if failures:
        print(f"{failures} selftest failure(s)")
        return 1
    print("selftest passed")
    return 0


COMMANDS = {
    "aggregates-programmed": (cmd_aggregates_programmed, 3),
    "scheduler-rates": (cmd_scheduler_rates, 2),
    "aggregate-detail": (cmd_aggregate_detail, 3),
    "session-view": (cmd_session_view, None),  # 1 or 2 args
    "pick-session-id": (cmd_pick_session_id, 1),
    "tin-metrics": (cmd_tin_metrics, 2),
    "no-sessions": (cmd_no_sessions, 1),
    "cli-scheduler-table": (cmd_cli_scheduler_table, 2),
    "cli-aggregate-tree": (cmd_cli_aggregate_tree, 2),
}


def main():
    if len(sys.argv) < 2:
        print(f"usage: {sys.argv[0]} <{'|'.join(sorted(COMMANDS))}|selftest> <file> [args]")
        return 2
    name = sys.argv[1]
    if name == "selftest":
        return cmd_selftest()
    entry = COMMANDS.get(name)
    if entry is None:
        print(f"unknown subcommand {name}")
        return 2
    fn, argc = entry
    args = sys.argv[2:]
    if argc is not None and len(args) != argc:
        print(f"{name} takes {argc} argument(s), got {len(args)}")
        return 2
    try:
        return fn(*args)
    except (KeyError, ValueError, TypeError, OSError, json.JSONDecodeError) as e:
        print(f"FAIL: {name}: {e.__class__.__name__}: {e}")
        return 1


if __name__ == "__main__":
    sys.exit(main())
