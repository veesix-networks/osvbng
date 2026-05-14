# test-infra/xl2tpd

Lightweight Debian + `xl2tpd` + `ppp` container, used as an L2TPv2 LAC
client in the `30-l2tp-lns` Robot suite. The image auto-dials a single
L2TP session to the address baked into the suite's `xl2tpd.conf` once
its access interface comes up, and starts pppd with CHAP — so the LNS
under test only has to handle the inbound side.

Not intended as a general-purpose LAC. It exists so the LNS suite has a
deterministic, scriptable external dialer (bngblaster does not support
acting as an L2TPv2 LAC).

## Build

```sh
docker build -t veesixnetworks/xl2tpd:local test-infra/xl2tpd/
```

Containerlab's `image-pull-policy: Never` keeps the local tag pinned in
the test runs.

## Configuration

The container expects three bind-mounted files in
`/etc/xl2tpd/xl2tpd.conf`, `/etc/xl2tpd/l2tp-secrets`, and
`/etc/ppp/options.xl2tpd` — supplied by the test under
`tests/30-l2tp-lns/config/lac/`. The entrypoint waits for `eth1`,
assigns the suite's static address, waits for L3 reachability to the
LNS, then starts xl2tpd in the foreground and issues `c lns` once so
the call kicks off without operator action.

## Logs

- xl2tpd runs in foreground; `docker logs` captures its stdout.
- pppd's `debug` output goes via syslog, surfaced through `rsyslogd`
  → `/var/log/syslog` which the entrypoint tails into stdout so all
  PPP exchanges land in `docker logs` too.
