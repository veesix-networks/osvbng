#!/bin/bash
# Copyright 2025 Veesix Networks Ltd
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

set -e
cd "$(dirname "$0")"

ACTION="${1:-start}"
shift || true

case "$ACTION" in
    start)
        BUILD=false
        HA_MODE=false
        while [ $# -gt 0 ]; do
            case "$1" in
                --build) BUILD=true ;;
                --ha) HA_MODE=true ;;
            esac
            shift
        done

        echo "Building osvbng..."
        (cd ../.. && make build)

        COMPOSE_ARGS=""
        if [ "$HA_MODE" = true ]; then
            COMPOSE_ARGS="--profile ha"
            echo "Starting containers (HA mode)..."
        else
            echo "Starting containers..."
        fi

        if [ "$BUILD" = true ]; then
            docker compose $COMPOSE_ARGS up -d --build
        else
            docker compose $COMPOSE_ARGS up -d
        fi

        echo "Waiting for containers..."
        until [ "$(docker inspect -f '{{.State.Running}}' osvbng-1 2>/dev/null)" = "true" ]; do sleep 1; done
        until [ "$(docker inspect -f '{{.State.Running}}' bng-blaster 2>/dev/null)" = "true" ]; do sleep 1; done
        until [ "$(docker inspect -f '{{.State.Running}}' frr-peer 2>/dev/null)" = "true" ]; do sleep 1; done
        if [ "$HA_MODE" = true ]; then
            until [ "$(docker inspect -f '{{.State.Running}}' osvbng-2 2>/dev/null)" = "true" ]; do sleep 1; done
        fi

        OSVBNG_PID=$(docker inspect -f '{{.State.Pid}}' osvbng-1)
        BLASTER_PID=$(docker inspect -f '{{.State.Pid}}' bng-blaster)
        FRRPEER_PID=$(docker inspect -f '{{.State.Pid}}' frr-peer)

        sudo ip link del osvbng-mgmt-br 2>/dev/null || true
        sudo ip link del osvbng1-mgmt-br 2>/dev/null || true
        sudo ip link del osvbng2-mgmt-br 2>/dev/null || true
        sudo ip link del osvbng-access 2>/dev/null || true
        sudo ip link del bng2-acc-br 2>/dev/null || true
        sudo ip link del osvbng-core 2>/dev/null || true
        sudo ip link del osvbng2-core 2>/dev/null || true

        echo "Creating management bridge..."
        sudo ip link add osvbng-mgmt-br type bridge
        sudo ip link set osvbng-mgmt-br up
        sudo ip addr add 172.30.0.1/24 dev osvbng-mgmt-br 2>/dev/null || true
        sudo sysctl -w net.ipv4.ip_forward=1 >/dev/null
        sudo iptables -t nat -A POSTROUTING -s 172.30.0.0/24 -j MASQUERADE 2>/dev/null || true

        echo "Connecting osvbng-1 to management bridge..."
        sudo ip link add osvbng1-mgmt type veth peer name osvbng1-mgmt-br
        sudo ip link set osvbng1-mgmt netns $OSVBNG_PID name eth0
        sudo ip link set osvbng1-mgmt-br master osvbng-mgmt-br
        sudo ip link set osvbng1-mgmt-br up
        docker exec osvbng-1 ip link set eth0 up
        docker exec osvbng-1 ip addr add 172.30.0.2/24 dev eth0
        docker exec osvbng-1 ip route add default via 172.30.0.1

        if [ "$HA_MODE" = true ]; then
            OSVBNG2_PID=$(docker inspect -f '{{.State.Pid}}' osvbng-2)

            echo "Connecting osvbng-2 to management bridge..."
            sudo ip link add osvbng2-mgmt type veth peer name osvbng2-mgmt-br
            sudo ip link set osvbng2-mgmt netns $OSVBNG2_PID name eth0
            sudo ip link set osvbng2-mgmt-br master osvbng-mgmt-br
            sudo ip link set osvbng2-mgmt-br up
            docker exec osvbng-2 ip link set eth0 up
            docker exec osvbng-2 ip addr add 172.30.0.3/24 dev eth0
            docker exec osvbng-2 ip route add default via 172.30.0.1
        fi

        echo "Creating access network (osvbng-1 <-> bng-blaster)..."
        sudo ip link add osvbng-access type veth peer name blaster-access
        sudo ip link set osvbng-access netns $OSVBNG_PID name eth1
        sudo ip link set blaster-access netns $BLASTER_PID name eth0
        docker exec osvbng-1 ip link set eth1 up
        docker exec bng-blaster ip link set eth0 up

        if [ "$HA_MODE" = true ]; then
            echo "Creating dummy access for osvbng-2..."
            sudo ip link add bng2-acc type veth peer name bng2-acc-br
            sudo ip link set bng2-acc netns $OSVBNG2_PID name eth1
            sudo ip link set bng2-acc-br up
            docker exec osvbng-2 ip link set eth1 up
        fi

        echo "Creating core network (osvbng-1 <-> frr-peer)..."
        sudo ip link add osvbng-core type veth peer name peer-core
        sudo ip link set osvbng-core netns $OSVBNG_PID name eth2
        sudo ip link set peer-core netns $FRRPEER_PID name eth0
        docker exec osvbng-1 ip link set eth2 up
        docker exec frr-peer ip link set eth0 up

        if [ "$HA_MODE" = true ]; then
            echo "Creating core network (osvbng-2 <-> frr-peer)..."
            sudo ip link add osvbng2-core type veth peer name peer-core1
            sudo ip link set osvbng2-core netns $OSVBNG2_PID name eth2
            sudo ip link set peer-core1 netns $FRRPEER_PID name eth1
            docker exec osvbng-2 ip link set eth2 up
            docker exec frr-peer ip link set eth1 up
        fi

        echo ""
        echo "Dev environment ready:"
        echo "  osvbng-1 (bng-1): 172.30.0.2 (8080, 9090, 50050)"
        if [ "$HA_MODE" = true ]; then
            echo "  osvbng-2 (bng-2): 172.30.0.3 (8080, 9090, 50050)"
        fi
        echo "  frr-peer:         10.0.0.2 (core link to osvbng-1)"
        if [ "$HA_MODE" = true ]; then
            echo "  frr-peer:         10.0.1.2 (core link to osvbng-2)"
        fi
        echo "  Prometheus:       http://localhost:9090"
        echo "  Grafana:          http://localhost:3000 (admin/admin)"
        ;;
    stop)
        docker compose --profile ha down
        sudo ip link del osvbng-mgmt-br 2>/dev/null || true
        sudo ip link del osvbng1-mgmt-br 2>/dev/null || true
        sudo ip link del osvbng2-mgmt-br 2>/dev/null || true
        sudo ip link del bng2-acc-br 2>/dev/null || true
        sudo ip link del osvbng-core 2>/dev/null || true
        sudo ip link del osvbng2-core 2>/dev/null || true
        ;;
    logs)
        docker compose --profile ha logs -f osvbng-1 osvbng-2
        ;;
    *)
        echo "Usage: $0 {start [--ha] [--build]|stop|logs}"
        exit 1
        ;;
esac
