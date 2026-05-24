#!/bin/sh
# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Sticky PPPoE subscriber for opdb-restore tests. Brings up the QinQ access
# interface for PPPoE discovery, then runs pppd under a respawn loop so the
# subscriber survives both BNG restarts and pppd's own holdoff/retry cycles.

set -eu

PHYSICAL_IFACE="${PHYSICAL_IFACE:-eth1}"
SVLAN="${SVLAN:-200}"
CVLAN="${CVLAN:-10}"
MGMT_IFACE="${MGMT_IFACE:-eth0}"

SVLAN_IFACE="${PHYSICAL_IFACE}.${SVLAN}"
TARGET_IFACE="${SVLAN_IFACE}.${CVLAN}"

log() { echo "subscribers: $*"; }

wait_for_iface() {
    _name="$1"
    _elapsed=0
    while [ ! -d "/sys/class/net/${_name}" ]; do
        if [ "$_elapsed" -ge 30 ]; then
            log "ERROR: ${_name} did not appear within 30s"
            exit 1
        fi
        sleep 1
        _elapsed=$((_elapsed + 1))
    done
}

log "Waiting for physical interface ${PHYSICAL_IFACE}"
wait_for_iface "${PHYSICAL_IFACE}"
ip link set "${PHYSICAL_IFACE}" up

if [ -n "${MGMT_IFACE}" ] && ip route show default dev "${MGMT_IFACE}" >/dev/null 2>&1; then
    log "Removing default route via ${MGMT_IFACE}"
    ip route del default dev "${MGMT_IFACE}" 2>/dev/null || true
fi

if ! ip link show "${SVLAN_IFACE}" >/dev/null 2>&1; then
    log "Creating S-VLAN ${SVLAN_IFACE} (dot1q ${SVLAN})"
    ip link add link "${PHYSICAL_IFACE}" name "${SVLAN_IFACE}" type vlan id "${SVLAN}"
fi
ip link set "${SVLAN_IFACE}" up

if ! ip link show "${TARGET_IFACE}" >/dev/null 2>&1; then
    log "Creating C-VLAN ${TARGET_IFACE} (dot1q ${CVLAN})"
    ip link add link "${SVLAN_IFACE}" name "${TARGET_IFACE}" type vlan id "${CVLAN}"
fi
ip link set "${TARGET_IFACE}" up

mkdir -p /var/log /etc/ppp/peers
chmod 600 /etc/ppp/peers/osvbng 2>/dev/null || true

log "Starting pppd call osvbng on ${TARGET_IFACE}"
exec pppd call osvbng
