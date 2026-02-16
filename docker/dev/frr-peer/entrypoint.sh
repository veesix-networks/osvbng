#!/bin/bash
set -e

echo "Waiting for eth0..."
WAIT_TIMEOUT=60
WAIT_COUNT=0
while [ ! -e /sys/class/net/eth0 ]; do
    sleep 1
    WAIT_COUNT=$((WAIT_COUNT + 1))
    if [ $WAIT_COUNT -ge $WAIT_TIMEOUT ]; then
        echo "ERROR: Timeout waiting for eth0"
        exit 1
    fi
done

echo "eth0 ready, creating L3VPN VRF..."
ip link add CUSTOMER-A type vrf table 100
ip link set CUSTOMER-A up
ip link add dummy-custa type dummy
ip link set dummy-custa master CUSTOMER-A
ip link set dummy-custa up
ip addr add 192.168.100.1/24 dev dummy-custa

echo "Configuring MPLS..."
sysctl -w net.mpls.platform_labels=1048575 || true
sysctl -w net.mpls.conf.lo.input=1 || true

echo "Starting FRR..."
/usr/lib/frr/frrinit.sh start

sleep 2

echo "Reloading FRR config (ensures ldpd picks up mpls ldp block)..."
/usr/lib/frr/frr-reload.py --reload /etc/frr/frr.conf 2>&1 || true

echo "FRR peer router started"
/usr/lib/frr/frrinit.sh status || true

exec tail -f /dev/null
