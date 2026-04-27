#!/bin/bash
# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

# Pattern A management VRF setup: pre-create the MGMT-VRF master and
# enslave the dedicated management NIC (eth3) before osvbng starts so
# the RADIUS plugin's first dial through netbind has a usable route
# inside the VRF table. vrfmgr.Reconcile recovers the existing master
# instead of creating its own.

set -e

VRF_NAME="MGMT-VRF"
VRF_TABLE=99
VRF_NIC=eth3
VRF_IPV4="10.99.0.2/24"

modprobe vrf 2>/dev/null || true

echo "Waiting for $VRF_NIC..."
for _ in $(seq 1 60); do
    if ip link show "$VRF_NIC" >/dev/null 2>&1; then
        break
    fi
    sleep 1
done

if ! ip link show "$VRF_NIC" >/dev/null 2>&1; then
    echo "ERROR: $VRF_NIC never appeared"
    ip link show
    exit 1
fi

echo "Creating $VRF_NAME master (table $VRF_TABLE) if needed..."
if ! ip link show "$VRF_NAME" >/dev/null 2>&1; then
    ip link add "$VRF_NAME" type vrf table "$VRF_TABLE"
fi
ip link set "$VRF_NAME" up

echo "Enslaving $VRF_NIC to $VRF_NAME and addressing $VRF_IPV4..."
ip link set "$VRF_NIC" master "$VRF_NAME"
ip addr replace "$VRF_IPV4" dev "$VRF_NIC"
ip link set "$VRF_NIC" up

echo "MGMT-VRF state:"
ip -d link show "$VRF_NAME"
ip -d link show "$VRF_NIC"
ip addr show "$VRF_NIC"
ip route show table "$VRF_TABLE" || true

exec /docker-entrypoint.sh "$@"
