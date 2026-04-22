#!/bin/bash
# Copyright 2025 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
OUT_DIR="${SCRIPT_DIR}/out"

if [ -z "$1" ]; then
    echo "Usage: $0 <test-suite-path>"
    echo "Example: $0 tests/01-smoke/"
    exit 1
fi

SUITE_PATH="$1"
shift
EXTRA_ARGS=("$@")

mkdir -p "${OUT_DIR}"

if [ ! -d "${SCRIPT_DIR}/.venv" ]; then
    echo "Creating Python virtual environment..."
    python3 -m venv "${SCRIPT_DIR}/.venv"
    source "${SCRIPT_DIR}/.venv/bin/activate"
    pip install -q robotframework robotframework-sshlibrary
else
    source "${SCRIPT_DIR}/.venv/bin/activate"
fi

function get_logname() {
    path=$1
    filename=$(basename "$path")
    if [[ "$filename" == *.* ]]; then
        dirname=$(dirname "$path")
        basename_noext=$(basename "$path" | cut -d. -f1)
        echo "${dirname##*/}-${basename_noext}"
    else
        echo "${filename}"
    fi
}

LOG_NAME=$(get_logname "${SUITE_PATH}")

echo "Running test suite: ${SUITE_PATH}"
echo "Output directory: ${OUT_DIR}"

robot \
    --consolecolors on \
    -r none \
    -l "${OUT_DIR}/${LOG_NAME}-log" \
    --output "${OUT_DIR}/${LOG_NAME}-out.xml" \
    "${EXTRA_ARGS[@]}" \
    "${SUITE_PATH}"
