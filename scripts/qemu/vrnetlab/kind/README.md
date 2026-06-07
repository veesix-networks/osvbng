# osvbng vrnetlab kind

This directory is the osvbng-specific kind drop-in for the upstream
vrnetlab build system. Don't run `make` from here directly — it
expects the upstream's `makefile.include` two levels up.

Use the orchestrator one level up instead:

```sh
../build.sh path/to/osvbng-debian12-v0.13.0.qcow2
```

That clones vrnetlab, drops this directory into it as `osvbng/`,
copies the qcow2 alongside, and runs the upstream Make targets.

## Files

- `Makefile` — tiny; sets `VENDOR` / `NAME` / `VERSION` regex; overrides
  `TAG_NAME` to `veesixnetworks/osvbng:ci-v<VERSION>` instead of the
  upstream default `vrnetlab/vr-osvbng:VERSION`.
- `docker/Dockerfile` — Debian 12 + qemu-kvm + bridge-utils + iproute2 +
  xorriso + python3. `ARG IMAGE` consumes the qcow2 the orchestrator drops
  next to it.
- `docker/launch.py` — subclasses `vrnetlab.VM`; renders a cloud-init
  seed ISO from `/config/osvbng.yaml` + an in-container-generated ssh
  keypair; overrides `bootstrap_spin()` to wait for ssh + `state=ready`
  rather than doing a console expect/login dance (cloud-init handles
  user setup, so there's nothing to script over serial).
- `docker/healthcheck.py` — Docker HEALTHCHECK probing the same
  ssh + state=ready.
