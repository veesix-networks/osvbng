#!/usr/bin/env bash
# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

# Build the vrnetlab-wrapped osvbng Docker image.
#
# Fetches the upstream vrnetlab tree (cached under $VRNETLAB_DIR,
# default /tmp/vrnetlab), drops our `kind/` files into
# $VRNETLAB_DIR/osvbng/, copies the qcow2 alongside, and runs the
# upstream Makefile. Result is tagged
# `veesixnetworks/osvbng:ci-v<X.Y.Z>` (or `ci-vlocal` if the qcow2
# filename has no version).

set -euo pipefail

usage() {
    cat >&2 <<EOF
Usage: $0 <qcow2-path>

Env vars:
  VRNETLAB_DIR   where to clone/find vrnetlab (default: /tmp/vrnetlab)
  VRNETLAB_REPO  upstream URL (default: https://github.com/vrnetlab/vrnetlab.git)
  VRNETLAB_REF   git ref to checkout (default: master). Pin for reproducibility.
EOF
    exit 1
}

[[ $# -eq 1 ]] || usage
QCOW2_PATH="$1"
[[ -f "$QCOW2_PATH" ]] || { echo "qcow2 not found: $QCOW2_PATH" >&2; exit 1; }

VRNETLAB_DIR="${VRNETLAB_DIR:-/tmp/vrnetlab}"
VRNETLAB_REPO="${VRNETLAB_REPO:-https://github.com/vrnetlab/vrnetlab.git}"
VRNETLAB_REF="${VRNETLAB_REF:-master}"

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KIND_SRC="$HERE/kind"
KIND_DST="$VRNETLAB_DIR/osvbng"

if [[ ! -d "$VRNETLAB_DIR/.git" ]]; then
    echo "==> Cloning $VRNETLAB_REPO -> $VRNETLAB_DIR"
    git clone --depth 1 --branch "$VRNETLAB_REF" "$VRNETLAB_REPO" "$VRNETLAB_DIR"
else
    echo "==> Reusing existing $VRNETLAB_DIR (set VRNETLAB_DIR to override)"
fi

echo "==> Staging osvbng kind into $KIND_DST"
mkdir -p "$KIND_DST"
cp -a "$KIND_SRC"/. "$KIND_DST"/
cp "$QCOW2_PATH" "$KIND_DST"/

echo "==> Building"
make -C "$KIND_DST"

echo
echo "Done. Image tagged. Verify with: docker images | grep osvbng | grep ci-"
