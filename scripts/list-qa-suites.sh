#!/bin/bash
# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

# Emit the runnable suite list as a JSON array: every tests/NN-*
# directory with a robot file, minus tests/skip-suites.txt. The
# integration workflow builds its matrix from this, so a new suite
# directory joins CI without touching any workflow yaml.
#
# Staging override: while tests/ci-suites.txt exists, CI runs exactly
# the suites it lists. Delete the file to return to the full
# generated matrix.
set -euo pipefail
cd "$(dirname "$0")/.."

if [ -f tests/ci-suites.txt ]; then
    sed 's/#.*//; s/[[:space:]]*$//' tests/ci-suites.txt | grep -v '^$' \
        | jq -R . | jq -sc .
    exit 0
fi

skips=$(sed 's/#.*//; s/[[:space:]]*$//' tests/skip-suites.txt)
for dir in tests/[0-9]*/; do
    s=$(basename "$dir")
    [ -f "tests/$s/$s.robot" ] || continue
    echo "$skips" | grep -qx "$s" && continue
    echo "$s"
done | jq -R . | jq -sc .
