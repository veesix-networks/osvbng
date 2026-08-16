# CI runner provisioning

Ansible provisioning for the dedicated bare-metal machine that runs
the osvbng and osvbng-vpp GitHub Actions runners. Everything here is
public and host-agnostic; everything host-specific (addresses, ports,
keys) lives in `inventory.yml`, which is gitignored. Copy
`inventory.example.yml` to `inventory.yml` and fill it in.

## Security model

The runner makes outbound HTTPS connections to GitHub only, so the
box needs no inbound ports at all. After bootstrap completes:

- The public interface default-denies all inbound except the
  WireGuard UDP port, which does not answer unauthenticated probes.
- sshd accepts keys only, no passwords, no root login, and after
  cutover listens on the WireGuard address only.
- fail2ban covers sshd for the bootstrap window while it is still
  publicly reachable.
- The runner executes jobs as an unprivileged `runner` user with
  passwordless sudo, because the suites need root (containerlab,
  docker). The box is single-purpose and burnable by design: keep
  nothing on it but the runner, and reprovision rather than repair.

Workflow runs from pull requests must stay approval-gated (GitHub
environment with a required reviewer) because approving a run
executes that PR's code on this machine.

## Bootstrap

The provider hands over an IP with an initial SSH key on port 22.

1. `cp inventory.example.yml inventory.yml` and fill it in. Leave
   `ssh_wg_only: false` for the first run.
2. Generate a WireGuard keypair for your workstation
   (`wg genkey | tee wg.key | wg pubkey`) and put the public key in
   the inventory. Keep `wg.key` out of the repo.
3. Fetch runner registration tokens (they expire after an hour):
   `gh api -X POST repos/veesix-networks/osvbng/actions/runners/registration-token -q .token`
   and the same for osvbng-vpp.
4. `ansible-playbook bootstrap.yml -e osvbng_runner_token=... -e osvbng_vpp_runner_token=...`
5. Add the server peer printed by the play to your workstation's
   WireGuard config, bring the tunnel up, and confirm
   `ssh runner@<wg address>` works.
6. Set `ssh_wg_only: true` in the inventory and rerun the playbook.
   sshd now binds to the tunnel and the public SSH port closes.

Rerunning the playbook is how drift gets fixed. Registration tasks
are skipped when the runner services already exist, so reruns do not
need tokens.

## Not covered here

Workflow definitions live in each repo's `.github/workflows/`. This
directory only builds the machine they run on.
