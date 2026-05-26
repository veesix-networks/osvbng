# Release Signing Keys

This directory holds the project's release-signing public key(s). The
corresponding private key is held by the maintainers and is never
committed to this repository.

## What lives here

- `cosign.pub` — the current project signing public key. Embedded into
  every QEMU image at packer build time as `/etc/osvbng/release-keys/cosign.pub`
  and used by `osvbngcli upgrade` to verify Tier A release tarballs.

## How verification works

Each release tarball ships with a detached signature file
(`osvbng-vX.Y.Z.tar.gz.sig`) produced by signing the tarball bytes with
the project's private key. The on-box verifier checks the signature
against `cosign.pub` before any host-mutating step. A tarball that fails
verification is quarantined to `/var/opt/osvbng/quarantine/` and the
upgrade is refused.

The current signature scheme is `cosign-v1-raw`: ASN.1-DER ECDSA / RSA-PSS /
Ed25519 over the tarball bytes, base64-encoded in the `.sig` file.
Future schemes (e.g. sigstore-keyless) will be added by introducing a
new sidecar filename (e.g. `.sigstore`); the `.sig` semantics here are
frozen for backward compatibility.

## Generating the keypair (one-time, maintainer-only)

```
scripts/release/generate-signing-key.sh
```

This produces:

- `release-keys/cosign.pub` — commit to the repo
- `cosign.key` (in the working directory) — **NEVER commit**. Move into
  the GitHub Actions secret store (or a hardware token / password
  manager interim) and delete the local copy.

## Rotating the key

Rotation has security implications and must be done deliberately:

1. Generate a new keypair (see above).
2. **Before retiring the old key**, ship a transitional image that
   embeds both `cosign.pub` (old) and `cosign-next.pub` (new). The
   verifier accepts a signature against either. This gives operators
   time to upgrade through the transition.
3. Once all supported deployments have rolled past the transitional
   image, ship a new image that embeds only the new key.

The transitional dual-key support requires the dispatch table in
`pkg/upgrade/signature.go` to be extended; it is not implemented
today (single key only).

## Self-builds

If you are building a QEMU image yourself (not the official Veesix
release pipeline) and you want signed-upgrade verification to work for
tarballs YOU sign, replace `cosign.pub` here with your own public key
and sign your tarballs with the matching private key. The official
Veesix-published `cosign.pub` cannot verify tarballs you sign with a
different key — that is the trust model working correctly.
