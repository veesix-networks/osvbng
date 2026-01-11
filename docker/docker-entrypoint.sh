#!/bin/bash
set -e

if [ "$1" = "config" ]; then
    exec /usr/local/bin/osvbngd "$@"
fi

if [ -z "$OSVBNG_ACCESS_INTERFACE" ]; then
    echo "ERROR: OSVBNG_ACCESS_INTERFACE environment variable is required"
    exit 1
fi

if [ -z "$OSVBNG_CORE_INTERFACE" ]; then
    echo "ERROR: OSVBNG_CORE_INTERFACE environment variable is required"
    exit 1
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

generate_default_configs() {
    mkdir -p /etc/osvbng
    mkdir -p /var/log/osvbng

    if [ ! -f /etc/osvbng/routing-daemons ]; then
        cat > /etc/osvbng/routing-daemons <<'EOF'
bgpd=yes
ospfd=yes
ospf6d=yes
bfdd=yes
ldpd=yes

vtysh_enable=yes
zebra_options="  -A 127.0.0.1 -s 67108864 -M dplane_fpm_nl"
bgpd_options="   -A 127.0.0.1"
ospfd_options="  -A 127.0.0.1"
ospf6d_options=" -A ::1"
staticd_options="-A 127.0.0.1"
bfdd_options="   -A 127.0.0.1"
ldpd_options="   -A 127.0.0.1"

log_file=/var/log/osvbng/routing.log
EOF
    fi

    if [ ! -f /etc/osvbng/routing.conf ]; then
        cat > /etc/osvbng/routing.conf <<'EOF'
hostname osvbng
log syslog informational
service integrated-vtysh-config
ip forwarding
ipv6 forwarding
!
line vty
!
end
EOF
    fi

    if [ ! -f /etc/osvbng/dataplane.conf ]; then
        cat > /etc/osvbng/dataplane.conf <<EOF
unix {
  interactive
  log /var/log/osvbng/dataplane.log
  full-coredump
  cli-listen /run/osvbng/cli.sock
  cli-prompt osvbng#
  cli-no-pager
  poll-sleep-usec 100
}

socksvr {
  socket-name /run/osvbng/dataplane_api.sock
}

api-trace {
  on
}

memory {
  main-heap-size 512M
  main-heap-page-size 4k
}

api-segment {
  gid osvbng
}

cpu {
  main-core ${OSVBNG_DP_MAIN_CORE}
$([ -n "$OSVBNG_DP_WORKER_CORES" ] && echo "  corelist-workers ${OSVBNG_DP_WORKER_CORES}")
}

buffers {
  buffers-per-numa 65536
  default data-size 2048
  page-size 4k
}

plugins {
  plugin default { enable }
  plugin dpdk_plugin.so { disable }
  plugin linux_cp_plugin.so { enable }
  plugin linux_nl_plugin.so { enable }
  plugin arp_plugin.so { disable }
  plugin rd_cp_plugin.so { disable }
  plugin igmp_plugin.so { disable }
  plugin v6n_osvbng_arp_punt_plugin.so { enable }
  plugin v6n_osvbng_fib_control_plugin.so { enable }
  plugin v6n_osvbng_accounting_plugin.so { enable }
}

logging {
  default-log-level info
  default-syslog-log-level info
}

linux-cp {
  lcp-sync
  lcp-auto-subint
  del-static-on-link-down
  del-dynamic-on-link-down
}

punt {
  socket /run/osvbng/punt.sock
}

statseg {
	default
	per-node-counters on
}
EOF
    fi

    if [ ! -f /etc/osvbng/osvbng.yaml ]; then
        echo "No configuration file found, generating default config..."
        /usr/local/bin/osvbngd config > /etc/osvbng/osvbng.yaml
        if [ $? -ne 0 ]; then
            echo "ERROR: Failed to generate configuration file"
            exit 1
        fi
        echo "Generated default configuration at /etc/osvbng/osvbng.yaml"
    fi
}

generate_default_configs

echo "Configuring hugepages..."
mkdir -p /dev/hugepages
mount -t hugetlbfs -o pagesize=2M none /dev/hugepages || true
echo 512 > /sys/kernel/mm/hugepages/hugepages-2048kB/nr_hugepages || true

echo "Using Docker-provided interface: $OSVBNG_ACCESS_INTERFACE"
ip link show $OSVBNG_ACCESS_INTERFACE

echo "Setting virtual MAC on $OSVBNG_ACCESS_INTERFACE..."
ip link set $OSVBNG_ACCESS_INTERFACE address 00:00:5e:00:01:01

echo "Creating runtime directories..."
mkdir -p /run/osvbng
chown root:osvbng /run/osvbng
chmod 770 /run/osvbng

echo "====== Linux Interfaces Below ======"
ip link show
echo "====== Linux Interfaces Above ======"

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

echo "Symlinking routing config to FRR directories..."
ln -sf /etc/osvbng/routing-daemons /etc/frr/daemons
ln -sf /etc/osvbng/routing.conf /etc/frr/frr.conf

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
