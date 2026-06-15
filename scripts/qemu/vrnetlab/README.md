# vrnetlab kind for osvbng

Lets containerlab run the packer-built `osvbng-debian12.qcow2` as a
node, transparently to the topology. Existing clab topology files and
Robot tests work against the wrapped VM the same way they work
against the Docker `veesixnetworks/osvbng:local` container — only the
image tag changes.

## What lives here

Only the osvbng-specific kind:

```
vrnetlab/
├── README.md       # this file
├── build.sh        # fetches vrnetlab, drops `kind/` into it, builds
└── kind/           # the bits that get copied into <vrnetlab>/osvbng/
    ├── Makefile
    ├── README.md
    └── docker/
        ├── Dockerfile
        ├── launch.py
        └── healthcheck.py
```

We do **not** vendor the upstream vrnetlab common library or makefile
includes — those are not ours to ship. `build.sh`
clones vrnetlab on demand and our kind drops in next to its peers
(`sros/`, `csr/`, …) using the upstream's own Makefile machinery.

## Build

```sh
./build.sh path/to/osvbng-debian12-v0.13.0.qcow2
```

That:

1. Clones `https://github.com/vrnetlab/vrnetlab.git` to `/tmp/vrnetlab`
   (cached — subsequent runs reuse it).
2. Copies `kind/*` into `/tmp/vrnetlab/osvbng/`.
3. Copies the qcow2 alongside.
4. Runs `make` in that directory.

Result: Docker image tagged `veesixnetworks/osvbng:ci-v<X.Y.Z>` (the
version is parsed from the qcow2 filename; if none is present, the tag
is `ci-vlocal`).

### Env knobs

| Var | Default | Purpose |
|--|--|--|
| `VRNETLAB_DIR` | `/tmp/vrnetlab` | Where to clone/find the upstream tree |
| `VRNETLAB_REPO` | `https://github.com/vrnetlab/vrnetlab.git` | Source repo (fork if needed) |
| `VRNETLAB_REF` | `master` | Branch/tag/commit to checkout — pin for reproducibility |

Example pinning to a known commit:

```sh
VRNETLAB_REF=abc1234 ./build.sh osvbng-debian12-v0.13.0.qcow2
```

## CI

When this gets wired into CI, the build step becomes:

```yaml
- name: build vrnetlab-wrapped osvbng
  run: |
    ./scripts/qemu/vrnetlab/build.sh out/osvbng-debian12-v${VERSION}.qcow2
```

CI can cache `/tmp/vrnetlab` between runs.

## Use from a clab topology

Take any existing clab topology that uses the Docker bng and change
two lines:

```yaml
bng1:
  kind: linux
  image: veesixnetworks/osvbng:ci-v0.13.0
  image-pull-policy: Never
  cap-add: [SYS_ADMIN, NET_ADMIN]
  env:
    OSVBNG_NUM_INTERFACES: "3"
  binds:
    - config/bng1/osvbng.yaml:/config/osvbng.yaml:ro
```

Requires `/dev/kvm` on the host. Boot time is ~60-90s per VM (vs
~3-5s for a Docker container), so reserve QEMU-mode runs for release
validation rather than every PR.