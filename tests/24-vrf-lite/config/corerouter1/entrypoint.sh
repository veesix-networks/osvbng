#!/bin/bash
# Copyright 2026 Veesix Networks Ltd
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

echo "eth1 ready, configuring VRF-lite topology..."
modprobe vrf 2>/dev/null || true
modprobe 8021q 2>/dev/null || true

if ip link add CUSTOMER-A type vrf table 200 2>/dev/null; then
    ip link set CUSTOMER-A up
    echo "CUSTOMER-A VRF created"
else
    echo "WARNING: CUSTOMER-A VRF already exists, continuing"
fi

if ip link add link eth1 name eth1.100 type vlan id 100 2>/dev/null; then
    ip link set eth1.100 up
    echo "eth1.100 created"
else
    echo "WARNING: eth1.100 already exists, continuing"
fi

if ip link add link eth1 name eth1.200 type vlan id 200 2>/dev/null; then
    ip link set eth1.200 master CUSTOMER-A
    ip link set eth1.200 up
    echo "eth1.200 created in CUSTOMER-A VRF"
else
    echo "WARNING: eth1.200 already exists, continuing"
fi

if ip link add dummy-default type dummy 2>/dev/null; then
    ip link set dummy-default up
    echo "dummy-default created"
else
    echo "WARNING: dummy-default already exists, continuing"
fi

if ip link add dummy-custa type dummy 2>/dev/null; then
    ip link set dummy-custa master CUSTOMER-A
    ip link set dummy-custa up
    echo "dummy-custa created in CUSTOMER-A VRF"
else
    echo "WARNING: dummy-custa already exists, continuing"
fi

touch /etc/frr/vtysh.conf

echo "Starting FRR..."
/usr/lib/frr/frrinit.sh start

sleep 2

echo "Reloading boot config..."
vtysh -b 2>&1 || true

echo "FRR VRF-lite router started"
/usr/lib/frr/frrinit.sh status || true
