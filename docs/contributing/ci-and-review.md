# CI and Review

This page describes what happens to a pull request after you open it:
which checks run, where they run, and what a reviewer looks for. If you
are adding an integration suite or changing the dataplane, the sections
on suite invariants and on osvbng-vpp are the ones that will save you a
round trip.

## What runs on your pull request

Two workflows run against a PR: hosted checks, and the integration
suites.

**CI** runs on GitHub hosted runners: `make build`, the unit tests, and a
changed-file summary. It is fast and it is the gate that fails most
often, usually on a compile error in a package you did not expect to
touch.

**integration** runs on the project's self-hosted rig. It deploys the
core suite set listed in `tests/ci-suites.txt`, containerlab topologies
driven by BNG Blaster and Robot Framework, against an image built from
your branch. It is the slower of the two by a wide margin.

**lint** gates what your branch introduces, rather than the whole tree,
so a finding it reports is one your change added. Run `make lint`
locally before you push and you will not meet it. Note that a local
`make lint` reports the repository's existing backlog as well as your
own findings; CI scopes to the diff.

!!! note "Fork pull requests wait for approval"
    A PR from a fork executes your branch's code on the shared rig, so
    its workflow runs land in the `action_required` state until a
    maintainer approves them. Nothing is wrong with your PR when you see
    that; it is waiting for a person. The same applies to every push you
    make afterwards.

## The three tiers

| Tier | Trigger | Scope | Purpose |
|------|---------|-------|---------|
| Per-PR (`integration`) | every PR to `main` | core set from `tests/ci-suites.txt` | fast signal before merge |
| Nightly (`nightly`) | scheduled daily, or manual dispatch | every suite `scripts/list-qa-suites.sh --all` generates, minus `tests/skip-suites.txt` | catch what the core set does not cover |
| Release qualification | manual, before a release | the full matrix, repeated | shake out flakes before shipping |

The per-PR set is a subset by choice, not by accident: suites run several
at a time, so the full matrix costs noticeably more wall clock than the
core set, and more again when PRs queue behind each other. The tradeoff
is that a change to a subsystem outside the core set gets no integration
signal on the PR itself. If your change touches one of those areas, run
the relevant suite locally and say so in the PR.

The nightly skips itself when nothing has changed. It compares the pair
(osvbng `main` commit, dataplane provenance commit) against the record
the last successful sweep wrote on the rig. A merge to osvbng-vpp alone
still triggers a sweep, because it changes what is under test.

## The rig

Suites need nested virtualisation, VPP hugepages and real interface
plumbing, so they run on project-maintained hardware rather than on
hosted runners. Build and unit tests, and the jobs that coordinate a
run, stay on hosted runners.

Capacity is finite and shared. Suite jobs across all tiers serialise on
a concurrency group, so one tier occupies the rig at a time and a second
run queues rather than interleaving. Several labs do run side by side,
which is why a suite has to tolerate neighbours: CI assigns each
concurrent lab its own CPU budget and passes it to the topology through
the environment (see the invariant below), so labs never contend for the
same VPP polling cores.

Robot's `output.xml` and `log.html` for every suite, plus container logs
from a failed one, are retained on the rig for a limited window. That copy
is the record rather than an uploaded artifact, because it survives a job
that dies before its steps finish, which is when you most want it. Ask a
maintainer if you need the evidence from a red CI run.

!!! warning "A timed-out suite reads as cancelled, not failed"
    Each suite job carries a time budget, the `suite_timeout` input in
    `.github/workflows/suite-run.yml`. GitHub reports a job that exceeds
    its `timeout-minutes` as **cancelled**, which looks like someone or
    something interrupted the run. Before hunting for an external cause,
    compare the job's duration against that budget: a job that ran for
    exactly the budget hit the timeout.

## Running the suites yourself

Build an image from your working tree and run one suite:

```bash
make docker-local
./scripts/run-qa-tests.sh -r 1 -t 03-ipoe-local
```

`make docker-local` installs the dataplane from `docker/dataplane-debs/`
when that directory holds debs, and falls back to the upstream fd.io
packages plus the prebuilt plugins in `test-infra/vpp-plugins/` when it
does not. To test against the same dataplane CI uses, stage the debs
first:

```bash
for p in 'vpp_*.deb' 'vpp-plugin-core_*.deb' \
         'vpp-plugin-dpdk_*.deb' 'libvppinfra_*.deb'; do
  gh release download dataplane-latest -R veesix-networks/osvbng-vpp \
    -p "$p" -D docker/dataplane-debs
done
```

Drop `-t <suite>` to run the whole set. Robot's `output.xml` and
`log.html` land in `tests/out/`.

## Writing an integration suite

New directories under `tests/` are picked up automatically by the
nightly matrix, so a suite that is not yet reliable belongs in
`tests/skip-suites.txt` with a reason, not merged as a red job. The
per-PR set is the separate allowlist in `tests/ci-suites.txt`.

Every topology carries a few invariants. Each exists because its absence
caused a real failure that took hours to attribute:

**Declare an IPv4-only management network.**

```yaml
mgmt:
  ipv4-subnet: 172.20.20.0/24
```

Without an explicit subnet, containerlab creates a dual-stack management
network, every node gets an IPv6 default route on `eth0`, and it races
the route the test's own Router Advertisement installs. Subscriber
originated IPv6 then leaves through management and dies, intermittently,
in a way that looks like a dataplane bug.

**Take the CPU slot from the environment.**

```yaml
env:
  OSVBNG_DP_WORKER_CORES: "${OSVBNG_LAB_WORKER_CORES:=auto}"
  OSVBNG_CP_CORES: "${OSVBNG_LAB_CP_CORES:=auto}"
```

CI sets those two variables to the budget it has allocated your lab. The
`:=auto` default matters: containerlab passes the unexpanded literal
through when a variable is empty, so "unset" has to arrive as a
non-empty sentinel that osvbng understands, and `auto` is the value that
tells it to size its own layout. Locally they are unset, so you get the
auto layout and nothing changes. A topology that omits this takes the
auto layout in CI too, sizing its dataplane for a machine it does not
have to itself, and starves every lab running beside it.

**Give shared network objects per-suite names.** Bridges, veth endpoint
names and management subnets that collide across suites break whichever
pair happens to run concurrently. If your topology uses static
management addresses, give it its own `network:` name and subnet too.

**Set `OSVBNG_RESPAWN: "true"`** on any node whose suite kills osvbngd,
so the entrypoint supervises and restarts it instead of the container
exiting.

**Gate restart verification on the state file.** After restarting
osvbngd, wait for `Wait For osvbng State Ready`, which polls
`/run/osvbng/state` for `ready`. Do not rely on the "osvbng started
successfully" log line alone: it survives from the previous boot, so a
health check that greps the container log passes instantly while opdb
recovery is still running, and the assertions that follow race it.

**Stay inside the suite time budget**, including deploy, session
establishment, any soak, and teardown. A suite that needs longer needs a
deliberate decision about that budget, not a silent timeout.

## What a reviewer looks for

**The PR body states the problem first.** The house format is problem,
then change, then verification, and the verification section says what
was *not* covered as well as what was. A reviewer reads the problem
statement to decide whether the change is the right shape; without it,
review turns into archaeology.

**Verification claims are reproducible.** "Suite 42 passes" is worth more
with the run or the summary line behind it. If a claim rests on a live
rig run, say which suites and how many times, particularly for anything
that has flaked before.

**Protocol code cites its source.** Behaviour that implements an RFC
names the section, in the code comment as well as the PR. Recollection
is not a source.

**Generated bindings match the dataplane under test.** If your change
regenerates `pkg/vpp/binapi/`, the message CRCs must match the plugin
the rig actually runs. Compare against
`/usr/share/vpp/api/plugins/<plugin>.api.json` in the built image; a
mismatch means the API landed in osvbng-vpp after the pin moved, or the
bindings were generated from an unmerged branch.

**New suites carry the invariants above**, and the topology has been run,
not just written.

**Conflicts are resolved by the author.** `main` moves, and a stale
branch that touches shared files (`tests/common.robot`,
`internal/ipoe/restore.go`, the docs) will conflict. Rebase rather than
leaving it for the person merging.

## Changing the dataplane

Plugin and VPP changes live in the `osvbng-vpp` repository, not here.
The flow is:

1. Open the PR against `osvbng-vpp`. Its build produces the deb set on
   the rig, seeds the box's per-commit artifact cache, and mirrors the
   debs to the rolling `dataplane-latest` prerelease with the source
   commit in the release notes.
2. If the change alters a `.api` file, regenerate the bindings in this
   repository and open a separate PR here.
3. Move the pin. `make bump-submodules` updates `vpp` and `context` to
   their current `main` and stages the result; commit that as its own
   focused PR. Pins move deliberately, so a checkout stays reproducible.

Anything that is about VPP itself rather than about osvbng goes upstream
to fd.io first, and lives in the patch queue only until it merges there.

## Practical notes

- Avoid opening or merging pull requests while a nightly sweep is
  running. PR runs and the sweep share the rig's concurrency group, and
  a run entering the queue can cancel one that is already executing.
- `go vet ./...` reports one pre-existing failure in
  `pkg/models/system`, a malformed struct tag that predates the current
  test suite. It is unrelated to your change.
- The plugin binaries committed under `test-infra/vpp-plugins/` are used
  only by the upstream-package image path. When `docker/dataplane-debs/`
  holds debs, the debs win and those binaries are ignored.
