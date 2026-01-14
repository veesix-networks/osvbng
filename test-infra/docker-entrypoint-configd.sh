#!/bin/bash
set -e

ACCESS_INTERFACE_LINUX=${ACCESS_INTERFACE_LINUX:-"eth0"}
CORE_INTERFACE_LINUX=${CORE_INTERFACE_LINUX:-"eth1"}

echo "Configuring hugepages..."
mkdir -p /dev/hugepages
mount -t hugetlbfs -o pagesize=2M none /dev/hugepages || true
echo 512 > /sys/kernel/mm/hugepages/hugepages-2048kB/nr_hugepages || true

echo "Using Docker-provided interface: $ACCESS_INTERFACE_LINUX"
ip link show $ACCESS_INTERFACE_LINUX

echo "Setting virtual MAC on $ACCESS_INTERFACE_LINUX..."
ip link set $ACCESS_INTERFACE_LINUX address 00:00:5e:00:01:01

echo "Creating VPP runtime directories..."
mkdir -p /run/vpp
chown root:vpp /run/vpp
chmod 770 /run/vpp

echo "====== Linux Interfaces Below ======"
ip link show
echo "====== Linux Interfaces Above ======"

echo "Starting VPP..."
/usr/bin/vpp -c /etc/vpp/startup.conf &
VPP_PID=$!

sleep 3

echo "Checking VPP process status..."
if ! kill -0 $VPP_PID 2>/dev/null; then
    echo "VPP process not running (PID $VPP_PID)"
    echo "====== VPP Log (last 50 lines) ======"
    tail -50 /var/log/vpp/vpp.log 2>/dev/null || echo "No log file found"
    exit 1
else
    echo "VPP process running (PID $VPP_PID)"
    echo "====== Checking if VPP API is responsive ======"
    vppctl show version && echo "VPP API responsive" || echo "VPP API not responding yet"
fi

echo "Waiting for VPP interfaces to be ready..."
sleep 1

echo "Starting FRRouting..."
/usr/lib/frr/frrinit.sh start

sleep 2

echo "Making zebra API socket accessible..."
chmod 660 /var/run/frr/zserv.api || true

echo "FRR Status:"
/usr/lib/frr/frrinit.sh status || true

echo "Starting osvbng with configd..."
echo "All interface/loopback provisioning will be done via configd"

exec taskset -c 3-7 /usr/local/bin/osvbngd -config /etc/bng/bng.yaml
