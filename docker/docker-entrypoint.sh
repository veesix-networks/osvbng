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
# nr_hugepages is the HOST-wide pool shared by every container on the
# box. Grow it if it is too small, never shrink it: a plain write of
# 512 here would pull the pool out from under labs already running.
HP_PATH=/sys/kernel/mm/hugepages/hugepages-2048kB/nr_hugepages
HP_WANT=512
HP_HAVE=$(cat $HP_PATH 2>/dev/null || echo 0)
if [ "$HP_HAVE" -lt "$HP_WANT" ]; then
    echo $HP_WANT > $HP_PATH || true
fi

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

export VPPCTL_SOCKET=/run/osvbng/cli.sock

# osvbngd connects to the API socket as soon as it starts and exits if
# the socket is missing, so the handover has to wait for the socket
# itself. A fixed sleep does not: it spent about eight seconds here
# regardless of what the dataplane was doing, and on a box running
# several labs at once VPP was still in plugin init when osvbngd gave
# up. osvbngd is PID 1, so its exit took the container with it, and the
# restart came back without the veths containerlab wired into the
# original netns, which turned a few seconds of startup lag into a
# permanently wedged node. Timeout over waiting forever: a dataplane
# that never opens the socket is broken, and it has to say so with its
# log rather than leave a silent restart loop behind.
DATAPLANE_API_SOCK=/run/osvbng/dataplane_api.sock
DATAPLANE_READY_TIMEOUT=${OSVBNG_DATAPLANE_READY_TIMEOUT:-60}

dataplane_failed() {
    echo "$1"
    echo "====== Dataplane Log (last 50 lines) ======"
    tail -50 /var/log/osvbng/dataplane.log 2>/dev/null || echo "No log file found"
    exit 1
}

echo "Waiting for dataplane API socket (timeout ${DATAPLANE_READY_TIMEOUT}s)..."
waited=0
while [ ! -S "$DATAPLANE_API_SOCK" ]; do
    if ! kill -0 $DATAPLANE_PID 2>/dev/null; then
        dataplane_failed "Dataplane process died during startup (PID $DATAPLANE_PID)"
    fi
    if [ "$waited" -ge "$DATAPLANE_READY_TIMEOUT" ]; then
        dataplane_failed "Dataplane API socket $DATAPLANE_API_SOCK did not appear after ${DATAPLANE_READY_TIMEOUT}s"
    fi
    sleep 1
    waited=$((waited + 1))
done
echo "Dataplane API socket ready after ${waited}s (PID $DATAPLANE_PID)"

echo "Setting dataplane API socket permissions..."
chmod 660 "$DATAPLANE_API_SOCK"
chown root:osvbng "$DATAPLANE_API_SOCK"

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

# "auto" is the same sentinel osvbngd's CPU resolution understands: it
# means "no explicit set, use the auto layout". The test topologies
# always pass the variable and default it to auto, because containerlab
# emits the unexpanded literal when a variable is empty, so an unset
# slot arrives here as the string auto rather than as an empty value.
# Taking it verbatim would run taskset -c auto, which fails to parse and
# leaves osvbngd unstarted; the resolved layout osvbngd already computed
# is what to pin to.
RESOLVED_CP="${OSVBNG_CP_CORES:-$OSVBNG_RESOLVED_CP_CORES}"
if [ "$RESOLVED_CP" = "auto" ]; then
    RESOLVED_CP="$OSVBNG_RESOLVED_CP_CORES"
fi

start_osvbngd() {
    if [ -n "$RESOLVED_CP" ]; then
        taskset -c ${RESOLVED_CP} /usr/local/bin/osvbngd -config /etc/osvbng/osvbng.yaml
    else
        /usr/local/bin/osvbngd -config /etc/osvbng/osvbng.yaml
    fi
}

# The console pty ships with XON/XOFF flow control enabled: one stray XOFF
# byte (a ctrl-S into an attached console) silently freezes every daemon's
# log output while the processes run on, and test gates that grep the logs
# hang on a healthy node. Nothing legitimate flow-controls a log sink.
if [ -t 1 ]; then
    stty -ixon -ixoff || true
fi

# OSVBNG_RESPAWN: test-only opt-in. When unset (prod default) the entrypoint
# exec's osvbngd as PID 1 so signals and exit codes propagate normally. When
# set to "true", osvbngd is supervised by this shell and respawned if it dies,
# so `pkill osvbngd` does not exit the container — used by opdb restore tests.
if [ "$OSVBNG_RESPAWN" = "true" ]; then
    echo "OSVBNG_RESPAWN=true: supervising osvbngd in a respawn loop"
    while true; do
        start_osvbngd || true
        echo "osvbngd exited, respawning in 2s..."
        sleep 2
    done
elif [ -n "$RESOLVED_CP" ]; then
    exec taskset -c ${RESOLVED_CP} /usr/local/bin/osvbngd -config /etc/osvbng/osvbng.yaml
else
    exec /usr/local/bin/osvbngd -config /etc/osvbng/osvbng.yaml
fi
