#!/bin/bash
# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

# Build a signed Tier A upgrade tarball from the current working tree.
# For iterating on osvbngcli upgrade in dev / lab boxes without cutting
# a real release.

set -e

usage() {
    cat >&2 <<EOF
Usage: $0 -k <cosign.key> -v <version> -o <output-dir>

  -k    path to cosign private key (decrypted, or set COSIGN_PASSWORD)
  -v    target version stamped into the binary via ldflags AND used
        as the manifest's osvbng_version (e.g. 0.13.0-rc1)
  -o    output dir for tarball + .sig
EOF
    exit 1
}

KEY="" VERSION="" OUTDIR=""
while getopts "k:v:o:" opt; do
    case "$opt" in
        k) KEY="$OPTARG" ;;
        v) VERSION="$OPTARG" ;;
        o) OUTDIR="$OPTARG" ;;
        *) usage ;;
    esac
done
[ -z "$KEY" ] || [ -z "$VERSION" ] || [ -z "$OUTDIR" ] && usage
[ ! -f "$KEY" ] && { echo "Key not found: $KEY" >&2; exit 1; }
command -v cosign >/dev/null || {
    echo "cosign not installed. Install with:" >&2
    echo "  go install github.com/sigstore/cosign/v2/cmd/cosign@latest" >&2
    exit 1
}

cd "$(dirname "$0")/../.."
REPO_ROOT="$(pwd)"
KEY=$(realpath "$KEY")
mkdir -p "$OUTDIR"
OUTDIR=$(realpath "$OUTDIR")

echo "==> Rebuilding with VERSION=$VERSION"
VERSION="$VERSION" make build >/dev/null

SHA_D=$(sha256sum bin/osvbngd | cut -d' ' -f1)
SHA_C=$(sha256sum bin/osvbngcli | cut -d' ' -f1)

STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT
cp bin/osvbngd "$STAGE/osvbngd"
cp bin/osvbngcli "$STAGE/osvbngcli"

cat > "$STAGE/manifest.yaml" <<EOF
osvbng_version: $VERSION
min_compatible_version: 0.0.0
type: A
build_date: $(date -u +%Y-%m-%dT%H:%M:%SZ)
build_commit: $(git rev-parse --short HEAD)
artifacts:
  - path: /usr/local/bin/osvbngd
    source: osvbngd
    sha256: $SHA_D
    mode: "0755"
    uid: 0
    gid: 0
  - path: /usr/local/bin/osvbngcli
    source: osvbngcli
    sha256: $SHA_C
    mode: "0755"
    uid: 0
    gid: 0
estimated_outage_seconds: 30
requires_reboot: false
EOF

TARBALL="$OUTDIR/osvbng-v$VERSION.tar.gz"
echo "==> Packing $TARBALL"
tar -czf "$TARBALL" -C "$STAGE" manifest.yaml osvbngd osvbngcli

echo "==> Signing"
cosign sign-blob --key "$KEY" --yes "$TARBALL" --output-signature "${TARBALL}.sig"

echo
echo "Done:"
echo "  $TARBALL"
echo "  ${TARBALL}.sig"
