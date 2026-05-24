#!/bin/sh
# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Sticky IPoE subscriber for opdb-restore tests. Brings up a QinQ access
# interface and runs dhclient v4 + v6 in the background, then sleeps so
# the container survives the BNG restart window without re-establishing.
#
# Intentionally tolerant of partial failures: any error past the
# interface-ready check is logged but does not exit the script, because
# the container would otherwise be restarted by docker (which tears down
# the veth peer to bng1 and stalls the whole test).

PHYSICAL_IFACE="${PHYSICAL_IFACE:-eth1}"
SVLAN="${SVLAN:-100}"
MGMT_IFACE="${MGMT_IFACE:-eth0}"

# Single-tag access. The double-tag (QinQ) path was tried first but the
# osvbng egress encodes the outer as 802.1ad (TPID 0x88a8) regardless of
# vlan-tpid: dot1q, while a Linux kernel VLAN interface stacked on a veth
# only receives 0x8100 outer. Single-tag is symmetric and exercises the
# opdb-restore code path equally well.
TARGET_IFACE="${PHYSICAL_IFACE}.${SVLAN}"

log() { echo "subscribers: $*"; }

wait_for_iface() {
    _name="$1"
    _elapsed=0
    while [ ! -d "/sys/class/net/${_name}" ]; do
        if [ "$_elapsed" -ge 60 ]; then
            log "WARN: ${_name} did not appear within 60s — sleeping anyway"
            return 1
        fi
        sleep 1
        _elapsed=$((_elapsed + 1))
    done
    return 0
}

log "Waiting for physical interface ${PHYSICAL_IFACE}"
wait_for_iface "${PHYSICAL_IFACE}" || exec sleep infinity
ip link set "${PHYSICAL_IFACE}" up || true

if [ -n "${MGMT_IFACE}" ] && ip route show default dev "${MGMT_IFACE}" >/dev/null 2>&1; then
    log "Removing default route via ${MGMT_IFACE}"
    ip route del default dev "${MGMT_IFACE}" 2>/dev/null || true
fi

if ! ip link show "${TARGET_IFACE}" >/dev/null 2>&1; then
    log "Creating access ${TARGET_IFACE} (dot1q ${SVLAN})"
    ip link add link "${PHYSICAL_IFACE}" name "${TARGET_IFACE}" type vlan id "${SVLAN}" || log "WARN: failed to create ${TARGET_IFACE}"
fi
ip link set "${TARGET_IFACE}" up || true

mkdir -p /var/log /var/lib/dhcp /var/run

log "Enabling IPv6 RA processing on ${TARGET_IFACE}"
sysctl -w "net.ipv6.conf.${TARGET_IFACE}.accept_ra=2" >/dev/null 2>&1 || true
sysctl -w "net.ipv6.conf.${TARGET_IFACE}.accept_ra_defrtr=1" >/dev/null 2>&1 || true

# Wait for the link-local v6 address to complete DAD. dhclient -6 binds to
# ff02::1:2 on the interface and fails with EADDRNOTAVAIL if the LL is still
# tentative — the existence of an fe80:: address alone is not sufficient.
_elapsed=0
while true; do
    _ll=$(ip -6 addr show dev "${TARGET_IFACE}" scope link 2>/dev/null)
    case "$_ll" in
        *fe80::*tentative*) ;;
        *fe80::*)           break ;;
    esac
    if [ "$_elapsed" -ge 20 ]; then
        log "WARN: link-local v6 still tentative on ${TARGET_IFACE} after 20s — proceeding anyway"
        break
    fi
    sleep 1
    _elapsed=$((_elapsed + 1))
done

log "Starting dhclient -4 on ${TARGET_IFACE}"
dhclient -4 -v -nw -cf /etc/dhcp/dhclient.conf \
    -lf /var/lib/dhcp/dhclient.${TARGET_IFACE}.leases \
    -pf /var/run/dhclient.${TARGET_IFACE}.pid \
    "${TARGET_IFACE}" >>/var/log/dhclient-v4.log 2>&1 || log "WARN: dhclient -4 returned $?"

# dhclient -6 can still race the LL completing — retry up to 5 times.
_attempt=0
while [ "$_attempt" -lt 5 ]; do
    log "Starting dhclient -6 on ${TARGET_IFACE} (attempt $((_attempt + 1)))"
    if dhclient -6 -v \
        -lf /var/lib/dhcp/dhclient.${TARGET_IFACE}.v6.leases \
        -pf /var/run/dhclient.${TARGET_IFACE}.v6.pid \
        -nw "${TARGET_IFACE}" >>/var/log/dhclient-v6.log 2>&1; then
        if [ -s /var/run/dhclient.${TARGET_IFACE}.v6.pid ] && kill -0 "$(cat /var/run/dhclient.${TARGET_IFACE}.v6.pid)" 2>/dev/null; then
            log "dhclient -6 daemonised successfully"
            break
        fi
    fi
    log "dhclient -6 failed, retrying in 3s"
    sleep 3
    _attempt=$((_attempt + 1))
done

log "Subscriber ready. Sleeping indefinitely so the container outlives BNG restarts."
exec sleep infinity
