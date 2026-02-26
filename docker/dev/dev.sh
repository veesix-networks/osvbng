#!/bin/bash
# Copyright 2025 Veesix Networks Ltd
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

set -e
cd "$(dirname "$0")"

case "${1:-start}" in
    start)
        echo "Building osvbng..."
        (cd ../.. && make build)

        echo "Starting containers..."
        if [ "$2" = "--build" ]; then
            docker compose up -d --build
        else
            docker compose up -d
        fi

        echo "Waiting for containers..."
        until [ "$(docker inspect -f '{{.State.Running}}' osvbng 2>/dev/null)" = "true" ]; do sleep 1; done
        until [ "$(docker inspect -f '{{.State.Running}}' osvbng-2 2>/dev/null)" = "true" ]; do sleep 1; done
        until [ "$(docker inspect -f '{{.State.Running}}' bng-blaster 2>/dev/null)" = "true" ]; do sleep 1; done
        until [ "$(docker inspect -f '{{.State.Running}}' frr-peer 2>/dev/null)" = "true" ]; do sleep 1; done

        OSVBNG_PID=$(docker inspect -f '{{.State.Pid}}' osvbng)
        OSVBNG2_PID=$(docker inspect -f '{{.State.Pid}}' osvbng-2)
        BLASTER_PID=$(docker inspect -f '{{.State.Pid}}' bng-blaster)
        FRRPEER_PID=$(docker inspect -f '{{.State.Pid}}' frr-peer)

        sudo ip link del osvbng-mgmt-br 2>/dev/null || true
        sudo ip link del osvbng1-mgmt-br 2>/dev/null || true
        sudo ip link del osvbng2-mgmt-br 2>/dev/null || true
        sudo ip link del osvbng-access 2>/dev/null || true
        sudo ip link del bng2-acc-br 2>/dev/null || true
        sudo ip link del osvbng-core 2>/dev/null || true

        echo "Creating management bridge..."
        sudo ip link add osvbng-mgmt-br type bridge
        sudo ip link set osvbng-mgmt-br up
        sudo ip addr add 172.30.0.1/24 dev osvbng-mgmt-br 2>/dev/null || true
        sudo sysctl -w net.ipv4.ip_forward=1 >/dev/null
        sudo iptables -t nat -A POSTROUTING -s 172.30.0.0/24 -j MASQUERADE 2>/dev/null || true

        echo "Connecting osvbng to management bridge..."
        sudo ip link add osvbng1-mgmt type veth peer name osvbng1-mgmt-br
        sudo ip link set osvbng1-mgmt netns $OSVBNG_PID name eth0
        sudo ip link set osvbng1-mgmt-br master osvbng-mgmt-br
        sudo ip link set osvbng1-mgmt-br up
        docker exec osvbng ip link set eth0 up
        docker exec osvbng ip addr add 172.30.0.2/24 dev eth0
        docker exec osvbng ip route add default via 172.30.0.1

        echo "Connecting osvbng-2 to management bridge..."
        sudo ip link add osvbng2-mgmt type veth peer name osvbng2-mgmt-br
        sudo ip link set osvbng2-mgmt netns $OSVBNG2_PID name eth0
        sudo ip link set osvbng2-mgmt-br master osvbng-mgmt-br
        sudo ip link set osvbng2-mgmt-br up
        docker exec osvbng-2 ip link set eth0 up
        docker exec osvbng-2 ip addr add 172.30.0.3/24 dev eth0
        docker exec osvbng-2 ip route add default via 172.30.0.1

        echo "Creating access network (osvbng <-> bng-blaster)..."
        sudo ip link add osvbng-access type veth peer name blaster-access
        sudo ip link set osvbng-access netns $OSVBNG_PID name eth1
        sudo ip link set blaster-access netns $BLASTER_PID name eth0
        docker exec osvbng ip link set eth1 up
        docker exec bng-blaster ip link set eth0 up

        echo "Creating dummy access for osvbng-2..."
        sudo ip link add bng2-acc type veth peer name bng2-acc-br
        sudo ip link set bng2-acc netns $OSVBNG2_PID name eth1
        sudo ip link set bng2-acc-br up
        docker exec osvbng-2 ip link set eth1 up

        echo "Creating core network (osvbng <-> frr-peer)..."
        sudo ip link add osvbng-core type veth peer name peer-core
        sudo ip link set osvbng-core netns $OSVBNG_PID name eth2
        sudo ip link set peer-core netns $FRRPEER_PID name eth0
        docker exec osvbng ip link set eth2 up
        docker exec frr-peer ip link set eth0 up

        echo ""
        echo "Dev environment ready:"
        echo "  osvbng (bng-1):   172.30.0.2 (8080, 9090, 50050, HA: 50051)"
        echo "  osvbng-2 (bng-2): 172.30.0.3 (8080, 9090, 50050, HA: 50051)"
        echo "  frr-peer:         10.0.0.2 (core link to osvbng)"
        echo "  Prometheus:       http://localhost:9090"
        echo "  Grafana:          http://localhost:3000 (admin/admin)"
        ;;
    stop)
        docker compose down
        sudo ip link del osvbng-mgmt-br 2>/dev/null || true
        sudo ip link del osvbng1-mgmt-br 2>/dev/null || true
        sudo ip link del osvbng2-mgmt-br 2>/dev/null || true
        sudo ip link del bng2-acc-br 2>/dev/null || true
        sudo ip link del osvbng-core 2>/dev/null || true
        ;;
    logs)
        docker compose logs -f osvbng osvbng-2
        ;;
    *)
        echo "Usage: $0 {start|stop|logs}"
        exit 1
        ;;
esac
