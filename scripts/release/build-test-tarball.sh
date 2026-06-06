#!/bin/bash
# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

set -euo pipefail

usage() {
    cat >&2 <<EOF
Usage: $0 -k <cosign.key> -v <version> -o <output-dir> [options]

Required:
  -k    path to cosign private key (decrypted, or set COSIGN_PASSWORD)
  -v    target version stamped into the binary via ldflags AND used
        as the manifest's osvbng_version (e.g. 0.14.0)
  -o    output dir for tarball + .sig

Options:
  --schema-version N       manifest schema_version (default: 2)
  --prev PATH              path to a previous tarball whose manifest is
                           embedded under prev/ and whose sha256 is recorded
                           in previous_manifest_sha256 (CI mode)
  --prev-version VERSION   shorthand: look up <output-dir>/osvbng-vVERSION.tar.gz
                           and use it as --prev (dev iteration mode)

Reproducibility:
  SOURCE_DATE_EPOCH        Unix timestamp; if set, every tar entry's mtime
                           is fixed and the resulting tarball is byte-stable
                           across rebuilds of the same commit.
EOF
    exit 1
}

KEY="" VERSION="" OUTDIR=""
SCHEMA_VERSION=2
PREV_PATH=""
PREV_VERSION=""

while [ $# -gt 0 ]; do
    case "$1" in
        -k) KEY="$2"; shift 2 ;;
        -v) VERSION="$2"; shift 2 ;;
        -o) OUTDIR="$2"; shift 2 ;;
        --schema-version) SCHEMA_VERSION="$2"; shift 2 ;;
        --prev) PREV_PATH="$2"; shift 2 ;;
        --prev-version) PREV_VERSION="$2"; shift 2 ;;
        -h|--help) usage ;;
        *) echo "unknown arg: $1" >&2; usage ;;
    esac
done

[ -z "$KEY" ] && usage
[ -z "$VERSION" ] && usage
[ -z "$OUTDIR" ] && usage
[ ! -f "$KEY" ] && { echo "Key not found: $KEY" >&2; exit 1; }
command -v cosign >/dev/null || {
    echo "cosign not installed. Install with:" >&2
    echo "  go install github.com/sigstore/cosign/v2/cmd/cosign@latest" >&2
    exit 1
}
command -v yq >/dev/null || {
    echo "yq not installed. Install with:" >&2
    echo "  sudo wget -O /usr/local/bin/yq https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64" >&2
    echo "  sudo chmod +x /usr/local/bin/yq" >&2
    exit 1
}

cd "$(dirname "$0")/../.."
REPO_ROOT="$(pwd)"
KEY=$(realpath "$KEY")
mkdir -p "$OUTDIR"
OUTDIR=$(realpath "$OUTDIR")

if [ -n "$PREV_VERSION" ] && [ -z "$PREV_PATH" ]; then
    PREV_PATH="$OUTDIR/osvbng-v$PREV_VERSION.tar.gz"
fi
if [ -n "$PREV_PATH" ] && [ ! -f "$PREV_PATH" ]; then
    echo "prev tarball not found: $PREV_PATH" >&2
    exit 1
fi

ARTIFACTS_YAML="$REPO_ROOT/release/artifacts.yaml"
[ ! -f "$ARTIFACTS_YAML" ] && { echo "$ARTIFACTS_YAML missing" >&2; exit 1; }

echo "==> Rebuilding with VERSION=$VERSION"
VERSION="$VERSION" make build >/dev/null

STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT

n=$(yq '.artifacts | length' "$ARTIFACTS_YAML")
for i in $(seq 0 $((n - 1))); do
    src=$(yq -r ".artifacts[$i].source" "$ARTIFACTS_YAML")
    if [ ! -f "$REPO_ROOT/$src" ]; then
        echo "artifacts.yaml references missing source: $src" >&2
        exit 1
    fi
    mkdir -p "$STAGE/$(dirname "$src")"
    cp "$REPO_ROOT/$src" "$STAGE/$src"
done

ARTIFACTS_RENDERED=""
for i in $(seq 0 $((n - 1))); do
    src=$(yq -r ".artifacts[$i].source" "$ARTIFACTS_YAML")
    install_path=$(yq -r ".artifacts[$i].install_path" "$ARTIFACTS_YAML")
    mode=$(yq -r ".artifacts[$i].mode" "$ARTIFACTS_YAML")
    uid=$(yq -r ".artifacts[$i].uid" "$ARTIFACTS_YAML")
    gid=$(yq -r ".artifacts[$i].gid" "$ARTIFACTS_YAML")
    rr=$(yq -r ".artifacts[$i].requires_restart" "$ARTIFACTS_YAML")
    sha=$(sha256sum "$STAGE/$src" | cut -d' ' -f1)
    ARTIFACTS_RENDERED+="  - path: $install_path
    source: $src
    sha256: $sha
    mode: \"$mode\"
    uid: $uid
    gid: $gid
    requires_restart: $rr
"
done

PREV_BLOCK=""
if [ -n "$PREV_PATH" ]; then
    PREV_STAGE=$(mktemp -d)
    tar -xzf "$PREV_PATH" -C "$PREV_STAGE" manifest.yaml
    if [ ! -f "${PREV_PATH}.sig" ]; then
        echo "prev tarball has no .sig: ${PREV_PATH}.sig" >&2
        rm -rf "$PREV_STAGE"
        exit 1
    fi
    mkdir -p "$STAGE/prev"
    cp "$PREV_STAGE/manifest.yaml" "$STAGE/prev/manifest.yaml"
    cp "${PREV_PATH}.sig" "$STAGE/prev/manifest.yaml.sig"
    PREV_VERSION_FOUND=$(yq -r '.osvbng_version' "$PREV_STAGE/manifest.yaml")
    PREV_SHA=$(sha256sum "$STAGE/prev/manifest.yaml" | cut -d' ' -f1)
    PREV_BLOCK="previous_version: $PREV_VERSION_FOUND
previous_manifest_sha256: $PREV_SHA
"
    rm -rf "$PREV_STAGE"
fi

BUILD_DATE=$(date -u -d "@${SOURCE_DATE_EPOCH:-$(date +%s)}" +%Y-%m-%dT%H:%M:%SZ)
BUILD_COMMIT=$(git rev-parse --short HEAD)

cat > "$STAGE/manifest.yaml" <<EOF
schema_version: $SCHEMA_VERSION
osvbng_version: $VERSION
min_compatible_version: 0.0.0
${PREV_BLOCK}type: A
build_date: $BUILD_DATE
build_commit: $BUILD_COMMIT
artifacts:
$ARTIFACTS_RENDERED
estimated_outage_seconds: 30
EOF

TARBALL="$OUTDIR/osvbng-v$VERSION.tar.gz"
echo "==> Packing $TARBALL"

MTIME=${SOURCE_DATE_EPOCH:-$(stat -c %Y "$STAGE/manifest.yaml")}
(
    cd "$STAGE"
    # Sort entries to give the tar a stable internal ordering. find -print0
    # then sort -z keeps newline-safe across weird paths (none expected
    # but defensively).
    find . -mindepth 1 -print0 |
        LC_ALL=C sort -z |
        tar --null --files-from=- \
            --owner=0 --group=0 --numeric-owner \
            --mtime="@$MTIME" \
            --sort=name \
            --format=ustar \
            -czf "$TARBALL"
)

echo "==> Signing"
cosign sign-blob --key "$KEY" --yes "$TARBALL" --output-signature "${TARBALL}.sig"

echo
echo "Done:"
echo "  $TARBALL"
echo "  ${TARBALL}.sig"
if [ -n "$PREV_PATH" ]; then
    echo "  (stepwise from $PREV_VERSION_FOUND, prev/manifest.yaml embedded)"
fi
