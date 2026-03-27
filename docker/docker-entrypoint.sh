#!/bin/bash
set -e

if [ "$1" = "config" ]; then
    exec /usr/local/bin/osvbngd "$@"
fi

if [ -z "$OSVBNG_MGMT_INTERFACE" ]; then
    OSVBNG_MGMT_INTERFACE="eth0"
fi

if [ -z "$OSVBNG_ACCESS_INTERFACE" ]; then
    OSVBNG_ACCESS_INTERFACE="eth1"
fi

wait_for_interfaces() {
    local interfaces=("$@")
    local timeout=300
    local elapsed=0

    echo "Waiting for interfaces to be provisioned: ${interfaces[*]}"

    while [ $elapsed -lt $timeout ]; do
        local all_present=true

        for iface in "${interfaces[@]}"; do
            if [ -z "$iface" ]; then
                continue
            fi

            if ! ip link show "$iface" >/dev/null 2>&1; then
                all_present=false
                break
            fi
        done

        if [ "$all_present" = true ]; then
            echo "All required interfaces are present"
            return 0
        fi

        echo "Waiting for interfaces... ($elapsed seconds elapsed)"
        sleep 5
        elapsed=$((elapsed + 5))
    done

    echo "ERROR: Timeout waiting for interfaces after $timeout seconds"
    echo "Available interfaces:"
    ip link show
    exit 1
}

if [ "$OSVBNG_WAIT_FOR_INTERFACES" = "true" ]; then
    WAIT_IFACES="$OSVBNG_MGMT_INTERFACE"
    OSVBNG_NUM_INTERFACES="${OSVBNG_NUM_INTERFACES:-2}"
    for i in $(seq 1 $((OSVBNG_NUM_INTERFACES - 1))); do
        WAIT_IFACES="$WAIT_IFACES eth$i"
    done
    wait_for_interfaces $WAIT_IFACES
fi

mkdir -p /etc/osvbng
mkdir -p /var/log/osvbng

echo "Configuring hugepages..."
mkdir -p /dev/hugepages
mount -t hugetlbfs -o pagesize=2M none /dev/hugepages || true
echo 512 > /sys/kernel/mm/hugepages/hugepages-2048kB/nr_hugepages || true

sysctl -w net.unix.max_dgram_qlen=10000 2>/dev/null || echo "Warning: Could not set max_dgram_qlen (need privileged mode)"
sysctl -w net.core.rmem_max=67108864 2>/dev/null || true
sysctl -w net.core.wmem_max=67108864 2>/dev/null || true

echo "Management interface: $OSVBNG_MGMT_INTERFACE"
ip link show $OSVBNG_MGMT_INTERFACE

echo "Creating runtime directories..."
mkdir -p /run/osvbng
chown root:osvbng /run/osvbng
chmod 770 /run/osvbng

echo "Creating dataplane network namespace..."
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
/usr/bin/vpp -c /etc/osvbng/dataplane.conf &
DATAPLANE_PID=$!

sleep 5

export VPPCTL_SOCKET=/run/osvbng/cli.sock

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
sleep 1

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

echo "Starting osvbng..."

RESOLVED_CP="${OSVBNG_CP_CORES:-$OSVBNG_RESOLVED_CP_CORES}"
if [ -n "$RESOLVED_CP" ]; then
    exec taskset -c ${RESOLVED_CP} /usr/local/bin/osvbngd -config /etc/osvbng/osvbng.yaml
else
    exec /usr/local/bin/osvbngd -config /etc/osvbng/osvbng.yaml
fi
