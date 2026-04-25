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

echo "Configuring MPLS..."
sysctl -w net.mpls.platform_labels=1048575 || true
sysctl -w net.mpls.conf.lo.input=1 || true
sysctl -w net.mpls.conf.eth1.input=1 || true
sysctl -w net.mpls.conf.eth2.input=1 || true

touch /etc/frr/vtysh.conf

echo "Starting FRR..."
/usr/lib/frr/frrinit.sh start

sleep 2

echo "Reloading boot config..."
vtysh -b 2>&1 || true

echo "FRR P router started"
/usr/lib/frr/frrinit.sh status || true
