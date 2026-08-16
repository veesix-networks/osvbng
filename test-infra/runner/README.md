# CI runner provisioning

Ansible provisioning for the machines that run the veesix-networks
GitHub Actions runners. Everything here is public and host-agnostic;
everything host-specific (addresses, ports, keys) lives in
`inventory.yml`, which is gitignored. Copy `inventory.example.yml`
to `inventory.yml` and fill it in.

## Security model

The runner makes outbound HTTPS connections to GitHub only, so the
box needs no inbound ports beyond SSH:

- Default-deny inbound; the only opening is the sshd listen port.
- sshd accepts keys only, no passwords, no root login, covered by
  fail2ban. `ssh_listen_port` is the port sshd listens on, which is
  not always the port the operator dials: an edge firewall may
  forward a public port to it. Directly exposed hosts can move sshd
  by changing this one variable; the firewall and fail2ban follow it.
- The runner executes jobs as an unprivileged `runner` user with
  passwordless sudo, because the suites need root (containerlab,
  docker). The box is single-purpose and burnable by design: keep
  nothing on it but the runner, and reprovision rather than repair.

Workflow runs from pull requests must stay approval-gated (GitHub
environment with a required reviewer) because approving a run
executes that PR's code on this machine.

## Runner pool

Runners register at the org level as one shared pool: any repo in
the org schedules jobs across all instances, and one workflow
fanning out into many jobs takes as many as are free. A runner
service executes one job at a time, so `runner_count` is the box's
total job parallelism.

Serialization is a workflow concern, not a pool concern: workflows
that must not overlap themselves (the osvbng-vpp release build with
its fixed volume and container names) declare a concurrency group,
and two runs of the same integration suite collide on containerlab
lab names, so suite jobs use a per-suite group (for example
`group: suite-${{ matrix.suite }}`). Perf jobs want a quiet machine
and take a box-wide group.

The org runner group must have "allow public repositories" enabled,
which is off by default; without it the pool sits idle for the
public repos.

## Bootstrap

1. `cp inventory.example.yml inventory.yml` and fill it in. The
   connecting user needs passwordless sudo.
2. Fetch an org registration token (expires after an hour):
   `gh api -X POST orgs/veesix-networks/actions/runners/registration-token -q .token`
3. `ansible-playbook bootstrap.yml -e runner_token=...`

Rerunning the playbook is how drift gets fixed. Registration is
one-shot per instance, so reruns do not need a token.

## Not covered here

Workflow definitions live in each repo's `.github/workflows/`. This
directory only builds the machine they run on.
