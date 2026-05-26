#!/bin/bash
# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

# Generates a fresh release-signing keypair for the project. Run this
# ONCE during initial setup (or during a deliberate key-rotation
# ceremony).
#
# Outputs:
#   release-keys/cosign.pub  — public key. Commit this to the repo.
#   cosign.key               — private key in the current directory.
#                              DO NOT commit. Move into your secret store
#                              and delete the local copy.

set -e

cd "$(dirname "$0")/../.."
REPO_ROOT="$(pwd)"
KEY_DIR="${REPO_ROOT}/release-keys"
PUB_KEY="${KEY_DIR}/cosign.pub"

if ! command -v cosign >/dev/null 2>&1; then
    echo "FATAL: cosign not installed. Install with:" >&2
    echo "  go install github.com/sigstore/cosign/v2/cmd/cosign@latest" >&2
    exit 1
fi

if [ -f "${PUB_KEY}" ]; then
    echo "ERROR: ${PUB_KEY} already exists." >&2
    echo "       Rotating the project signing key is a deliberate ceremony." >&2
    echo "       See release-keys/README.md before proceeding." >&2
    exit 1
fi

if [ -f "${REPO_ROOT}/cosign.key" ]; then
    echo "ERROR: ${REPO_ROOT}/cosign.key already exists. Move it out of the" >&2
    echo "       repo before generating a new keypair." >&2
    exit 1
fi

mkdir -p "${KEY_DIR}"

echo "Generating cosign keypair in ${REPO_ROOT}..."
echo "You will be prompted for a passphrase to protect the private key."
echo

cd "${REPO_ROOT}"
cosign generate-key-pair

mv "${REPO_ROOT}/cosign.pub" "${PUB_KEY}"

cat <<EOF

Keypair generated.

  Public:   ${PUB_KEY}
  Private:  ${REPO_ROOT}/cosign.key

NEXT STEPS:

  1. Commit the public key:
       git add ${PUB_KEY#${REPO_ROOT}/}
       git commit -m "feat(release): add project signing public key"

  2. Move the private key into a secret store. Recommended:
       - Add as a GitHub Actions secret named COSIGN_PRIVATE_KEY
       - Also keep an encrypted backup somewhere your team can recover from

  3. Delete the local copy:
       shred -u ${REPO_ROOT}/cosign.key

  4. Test the signing flow on a release tarball:
       cosign sign-blob --key cosign.key path/to/osvbng-vX.Y.Z.tar.gz \\
           --output-signature path/to/osvbng-vX.Y.Z.tar.gz.sig

The QEMU image build will fail (intentionally) until the public key
is committed; that prevents shipping an image without a trust anchor.
EOF
