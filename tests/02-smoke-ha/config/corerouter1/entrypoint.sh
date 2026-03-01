#!/bin/bash
# Copyright 2025 Veesix Networks Ltd
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

set -e

echo "Waiting for eth1..."
WAIT_TIMEOUT=60
WAIT_COUNT=0
while [ ! -e /sys/class/net/eth1 ]; do
    sleep 1
    WAIT_COUNT=$((WAIT_COUNT + 1))
    if [ $WAIT_COUNT -ge $WAIT_TIMEOUT ]; then
        echo "ERROR: Timeout waiting for eth1"
        exit 1
    fi
done

echo "Waiting for eth2..."
WAIT_COUNT=0
while [ ! -e /sys/class/net/eth2 ]; do
    sleep 1
    WAIT_COUNT=$((WAIT_COUNT + 1))
    if [ $WAIT_COUNT -ge $WAIT_TIMEOUT ]; then
        echo "ERROR: Timeout waiting for eth2"
        exit 1
    fi
done

echo "Interfaces ready, creating L3VPN VRF..."
modprobe vrf 2>/dev/null || true
if ip link add CUSTOMER-A type vrf table 100 2>/dev/null; then
    ip link set CUSTOMER-A up
    ip link add dummy-custa type dummy
    ip link set dummy-custa master CUSTOMER-A
    ip link set dummy-custa up
    ip addr add 192.168.100.1/24 dev dummy-custa
    echo "L3VPN VRF created"
else
    echo "WARNING: VRF creation not supported, skipping L3VPN setup"
fi

echo "Configuring MPLS..."
sysctl -w net.mpls.platform_labels=1048575 || true
sysctl -w net.mpls.conf.lo.input=1 || true

touch /etc/frr/vtysh.conf

echo "Starting FRR..."
/usr/lib/frr/frrinit.sh start

sleep 2

echo "Reloading boot config..."
vtysh -b 2>&1 || true

echo "FRR core router started"
/usr/lib/frr/frrinit.sh status || true
