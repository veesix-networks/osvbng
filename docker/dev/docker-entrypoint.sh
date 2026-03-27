#!/bin/bash
set -e

# Wait for dataplane interfaces (eth1 through eth$(NUM-1))
# eth0 is always management, eth1+ are dataplane
OSVBNG_NUM_INTERFACES="${OSVBNG_NUM_INTERFACES:-2}"
OSVBNG_DP_INTERFACES=$((OSVBNG_NUM_INTERFACES - 1))
echo "Waiting for $OSVBNG_DP_INTERFACES dataplane interface(s)..."

WAIT_TIMEOUT=60
WAIT_COUNT=0
while true; do
    FOUND=0
    for i in $(seq 1 $OSVBNG_DP_INTERFACES); do
        if [ -e "/sys/class/net/eth$i" ]; then
            FOUND=$((FOUND + 1))
        fi
    done

    if [ $FOUND -eq $OSVBNG_DP_INTERFACES ]; then
        echo "All $OSVBNG_DP_INTERFACES dataplane interface(s) ready"
        break
    fi

    sleep 1
    WAIT_COUNT=$((WAIT_COUNT + 1))
    if [ $WAIT_COUNT -ge $WAIT_TIMEOUT ]; then
        echo "ERROR: Timeout waiting for dataplane interfaces (found $FOUND of $OSVBNG_ACCESS_INTERFACES)"
        exit 1
    fi
done

# First dataplane interface is the primary access interface
OSVBNG_ACCESS_INTERFACE="eth1"

mkdir -p /etc/osvbng
mkdir -p /var/log/osvbng

echo "Configuring hugepages..."
mkdir -p /dev/hugepages
mount -t hugetlbfs -o pagesize=2M none /dev/hugepages || true
echo 512 > /sys/kernel/mm/hugepages/hugepages-2048kB/nr_hugepages || true

echo "Using dataplane interface: $OSVBNG_ACCESS_INTERFACE"
ip link show $OSVBNG_ACCESS_INTERFACE

echo "Creating runtime directories..."
mkdir -p /run/osvbng
chown root:osvbng /run/osvbng
chmod 770 /run/osvbng

echo "Creating dataplane network namespace (available for DPDK deployments)..."
ip netns add dataplane || true
ip netns exec dataplane ip link set lo up

echo "====== Linux Interfaces Below ======"
ip link show
echo "====== Linux Interfaces Above ======"

echo "Generating external configurations..."
/usr/local/bin/osvbngd -config /etc/osvbng/osvbng.yaml generate-external
if [ $? -ne 0 ]; then
    echo "ERROR: Failed to generate external configurations"
    exit 1
fi

if [ -f /run/osvbng/cpu-layout.env ]; then
    source /run/osvbng/cpu-layout.env
    echo "Resolved CPU layout: main=$OSVBNG_RESOLVED_MAIN_CORE workers=$OSVBNG_RESOLVED_WORKER_CORES cp=$OSVBNG_RESOLVED_CP_CORES total=$OSVBNG_RESOLVED_TOTAL_CORES"
fi

echo "Starting dataplane..."
/usr/bin/vpp -c /etc/osvbng/dataplane.conf > /var/log/osvbng/dataplane-stderr.log 2>&1 &
DATAPLANE_PID=$!

echo "Checking dataplane process status..."
if ! kill -0 $DATAPLANE_PID 2>/dev/null; then
    echo "Dataplane process not running (PID $DATAPLANE_PID)"
    echo "====== Dataplane Log (last 50 lines) ======"
    tail -50 /var/log/osvbng/dataplane.log 2>/dev/null || echo "No log file found"
    exit 1
else
    echo "Dataplane process running (PID $DATAPLANE_PID)"
    echo "====== Checking if dataplane API is responsive ======"
    vppctl -s /run/osvbng/cli.sock show version && echo "Dataplane API responsive" || echo "Dataplane API not responding yet"
fi

echo "Waiting for dataplane interfaces to be ready..."
sleep 5

vppctl -s /run/osvbng/cli.sock show interface || echo "Warning: VPP not ready yet, continuing..."

echo "Setting dataplane API socket permissions..."
chmod 660 /run/osvbng/dataplane_api.sock || true
chown root:osvbng /run/osvbng/dataplane_api.sock || true

echo "Configuring kernel MPLS in dataplane namespace..."
ip netns exec dataplane sysctl -w net.mpls.platform_labels=1048575 || true
ip netns exec dataplane sysctl -w net.mpls.conf.lo.input=1 || true

echo "Linking FRR configs to osvbng directory..."
ln -sf /etc/osvbng/routing-daemons /etc/frr/daemons
ln -sf /etc/osvbng/frr.conf /etc/frr/frr.conf

echo "Starting routing daemons in dataplane namespace..."
ip netns exec dataplane /usr/lib/frr/frrinit.sh start

sleep 2

echo "Making zebra API socket accessible..."
chmod 660 /var/run/frr/zserv.api || true

echo "Routing daemon status:"
ip netns exec dataplane /usr/lib/frr/frrinit.sh status || true

sleep 5

if ! kill -0 $DATAPLANE_PID 2>/dev/null; then
    echo "ERROR: VPP crashed before osvbng could start"
    echo "====== VPP stderr ======"
    cat /var/log/osvbng/dataplane-stderr.log 2>/dev/null
    echo "====== dmesg (last 20) ======"
    dmesg | tail -20 2>/dev/null || true
    exit 1
fi

echo "Starting osvbng..."

RESOLVED_CP="${OSVBNG_CP_CORES:-$OSVBNG_RESOLVED_CP_CORES}"
if [ -n "$RESOLVED_CP" ]; then
    exec taskset -c ${RESOLVED_CP} /usr/local/bin/osvbngd -config /etc/osvbng/osvbng.yaml
else
    exec /usr/local/bin/osvbngd -config /etc/osvbng/osvbng.yaml
fi
