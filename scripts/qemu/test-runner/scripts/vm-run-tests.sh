#!/bin/bash
# Copyright 2025 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

set -euo pipefail

QCOW2="${1:?Usage: vm-run-tests.sh <qcow2> <docker-tar> <test-suite> [output-dir] [extra-image-tar...]}"
DOCKER_TAR="${2:?Missing docker tarball}"
TEST_SUITE="${3:?Missing test suite (e.g. 01-smoke)}"
OUTPUT_DIR="${4:-./test-results}"
shift 4 2>/dev/null || shift $#
EXTRA_IMAGES=("$@")

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SSH_PORT=2222

mkdir -p "${OUTPUT_DIR}"

cleanup() {
  echo "==> Collecting test results from VM..."
  "${SCRIPT_DIR}/vm-exec.sh" "mkdir -p /tmp/test-artifacts && for c in \$(docker ps -a --filter 'label=containerlab' --format '{{.Names}}'); do docker logs \"\$c\" > /tmp/test-artifacts/\${c}.log 2>&1 || true; done" "${SSH_PORT}" || true
  "${SCRIPT_DIR}/vm-copy.sh" from "/root/tests/out" "${OUTPUT_DIR}/" "${SSH_PORT}" || true
  "${SCRIPT_DIR}/vm-copy.sh" from "/tmp/test-artifacts" "${OUTPUT_DIR}/" "${SSH_PORT}" || true

  echo "==> Destroying containerlab topology..."
  "${SCRIPT_DIR}/vm-exec.sh" "containerlab destroy --all --cleanup 2>/dev/null" "${SSH_PORT}" || true

  echo "==> Stopping VM..."
  "${SCRIPT_DIR}/vm-stop.sh" "${SSH_PORT}"
}
trap cleanup EXIT

echo "==> Starting VM..."
"${SCRIPT_DIR}/vm-start.sh" "${QCOW2}" "${SSH_PORT}"

echo "==> Waiting for SSH..."
"${SCRIPT_DIR}/vm-wait-ssh.sh" "${SSH_PORT}" 300

echo "==> Loading kernel modules..."
"${SCRIPT_DIR}/vm-exec.sh" "modprobe vrf mpls_router mpls_iptunnel dummy" "${SSH_PORT}"
"${SCRIPT_DIR}/vm-exec.sh" "sysctl -w net.mpls.platform_labels=1048575" "${SSH_PORT}"

echo "==> Copying Docker image to VM..."
"${SCRIPT_DIR}/vm-copy.sh" to "${DOCKER_TAR}" "/tmp/osvbng-local.tar" "${SSH_PORT}"

echo "==> Loading Docker image in VM..."
"${SCRIPT_DIR}/vm-exec.sh" "docker load -i /tmp/osvbng-local.tar && rm -f /tmp/osvbng-local.tar" "${SSH_PORT}"

for extra_tar in "${EXTRA_IMAGES[@]}"; do
  if [ -n "${extra_tar}" ] && [ -f "${extra_tar}" ]; then
    extra_name="$(basename "${extra_tar}")"
    echo "==> Copying extra image ${extra_name} to VM..."
    "${SCRIPT_DIR}/vm-copy.sh" to "${extra_tar}" "/tmp/${extra_name}" "${SSH_PORT}"
    echo "==> Loading extra image ${extra_name} in VM..."
    "${SCRIPT_DIR}/vm-exec.sh" "docker load -i /tmp/${extra_name} && rm -f /tmp/${extra_name}" "${SSH_PORT}"
  fi
done

echo "==> Copying tests to VM..."
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"
"${SCRIPT_DIR}/vm-copy.sh" to "${REPO_ROOT}/tests" "/root/tests" "${SSH_PORT}"

echo "==> Running ${TEST_SUITE} tests..."
"${SCRIPT_DIR}/vm-exec.sh" "cd /root/tests && mkdir -p out && robot --consolecolors on -r none -l ./out/${TEST_SUITE}-log --output ./out/output.xml ${ROBOT_EXTRA_ARGS:-} ./${TEST_SUITE}/" "${SSH_PORT}"
