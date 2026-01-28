#!/bin/bash
set -e

if [ -z "$OSVBNG_ACCESS_INTERFACE" ]; then
    for iface in $(ls /sys/class/net/ | grep eth); do
        if ip link show "$iface" 2>/dev/null | grep -q "bng-access"; then
            OSVBNG_ACCESS_INTERFACE="$iface"
            break
        fi
    done
    OSVBNG_ACCESS_INTERFACE="${OSVBNG_ACCESS_INTERFACE:-eth1}"
fi

TOTAL_CORES=$(nproc)

if [ -z "$OSVBNG_DP_MAIN_CORE" ] || [ -z "$OSVBNG_DP_WORKER_CORES" ] || [ -z "$OSVBNG_CP_CORES" ]; then
    case $TOTAL_CORES in
        1)
            OSVBNG_DP_MAIN_CORE=0
            OSVBNG_DP_WORKER_CORES=""
            OSVBNG_CP_CORES=""
            USE_TASKSET=false
            ;;
        2)
            OSVBNG_DP_MAIN_CORE=0
            OSVBNG_DP_WORKER_CORES=1
            OSVBNG_CP_CORES=0
            USE_TASKSET=true
            ;;
        3)
            OSVBNG_DP_MAIN_CORE=0
            OSVBNG_DP_WORKER_CORES=1-2
            OSVBNG_CP_CORES=0
            USE_TASKSET=true
            ;;
        4)
            OSVBNG_DP_MAIN_CORE=0
            OSVBNG_DP_WORKER_CORES=1-3
            OSVBNG_CP_CORES=0
            USE_TASKSET=true
            ;;
        *)
            OSVBNG_DP_MAIN_CORE=0
            OSVBNG_DP_WORKER_CORES=1-$((TOTAL_CORES-2))
            OSVBNG_CP_CORES=$((TOTAL_CORES-1))
            USE_TASKSET=true
            ;;
    esac
fi

echo "Core allocation: Total=$TOTAL_CORES DP_MAIN=$OSVBNG_DP_MAIN_CORE DP_WORKERS=$OSVBNG_DP_WORKER_CORES CP=$OSVBNG_CP_CORES"

mkdir -p /etc/osvbng
mkdir -p /var/log/osvbng

echo "Configuring hugepages..."
mkdir -p /dev/hugepages
mount -t hugetlbfs -o pagesize=2M none /dev/hugepages || true
echo 512 > /sys/kernel/mm/hugepages/hugepages-2048kB/nr_hugepages || true

echo "Using Docker-provided interface: $OSVBNG_ACCESS_INTERFACE"
ip link show $OSVBNG_ACCESS_INTERFACE

echo "Creating runtime directories..."
mkdir -p /run/osvbng
chown root:osvbng /run/osvbng
chmod 770 /run/osvbng

echo "====== Linux Interfaces Below ======"
ip link show
echo "====== Linux Interfaces Above ======"

echo "Generating external configurations..."
/usr/local/bin/osvbngd -config /etc/osvbng/osvbng.yaml generate-external
if [ $? -ne 0 ]; then
    echo "ERROR: Failed to generate external configurations"
    exit 1
fi

echo "Starting dataplane..."
/usr/bin/vpp -c /etc/osvbng/dataplane.conf &
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

echo "Linking FRR configs to osvbng directory..."
ln -sf /etc/osvbng/routing-daemons /etc/frr/daemons
ln -sf /etc/osvbng/frr.conf /etc/frr/frr.conf

echo "Starting routing daemons..."
/usr/lib/frr/frrinit.sh start

sleep 2

echo "Making zebra API socket accessible..."
chmod 660 /var/run/frr/zserv.api || true

echo "Routing daemon status:"
/usr/lib/frr/frrinit.sh status || true

echo "Starting osvbng..."

if [ "$USE_TASKSET" = true ] && [ -n "$OSVBNG_CP_CORES" ]; then
    exec taskset -c ${OSVBNG_CP_CORES} /usr/local/bin/osvbngd -config /etc/osvbng/osvbng.yaml
else
    exec /usr/local/bin/osvbngd -config /etc/osvbng/osvbng.yaml
fi
