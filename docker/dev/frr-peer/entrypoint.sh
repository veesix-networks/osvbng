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

echo "eth0 ready, starting FRR..."
/usr/lib/frr/frrinit.sh start

echo "FRR peer router started"
/usr/lib/frr/frrinit.sh status || true

exec tail -f /dev/null
