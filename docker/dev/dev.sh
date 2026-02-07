#!/bin/bash
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
        until [ "$(docker inspect -f '{{.State.Running}}' bng-blaster 2>/dev/null)" = "true" ]; do sleep 1; done

        OSVBNG_PID=$(docker inspect -f '{{.State.Pid}}' osvbng)
        BLASTER_PID=$(docker inspect -f '{{.State.Pid}}' bng-blaster)

        sudo ip link del osvbng-mgmt 2>/dev/null || true
        sudo ip link del osvbng-access 2>/dev/null || true

        echo "Creating management network..."
        sudo ip link add osvbng-mgmt type veth peer name osvbng-mgmt-br
        sudo ip link set osvbng-mgmt netns $OSVBNG_PID name eth0
        sudo ip link set osvbng-mgmt-br up
        docker exec osvbng ip link set eth0 up
        docker exec osvbng ip addr add 172.30.0.2/24 dev eth0
        docker exec osvbng ip route add default via 172.30.0.1
        sudo ip addr add 172.30.0.1/24 dev osvbng-mgmt-br 2>/dev/null || true
        sudo sysctl -w net.ipv4.ip_forward=1 >/dev/null
        sudo iptables -t nat -A POSTROUTING -s 172.30.0.0/24 -j MASQUERADE 2>/dev/null || true

        echo "Creating access network..."
        sudo ip link add osvbng-access type veth peer name blaster-access
        sudo ip link set osvbng-access netns $OSVBNG_PID name eth1
        sudo ip link set blaster-access netns $BLASTER_PID name eth0
        docker exec osvbng ip link set eth1 up
        docker exec bng-blaster ip link set eth0 up

        echo ""
        echo "Dev environment ready:"
        echo "  osvbng: 172.30.0.2 (8080, 9090, 50050)"
        echo "  Prometheus: http://localhost:9090"
        echo "  Grafana: http://localhost:3000 (admin/admin)"
        ;;
    stop)
        docker compose down
        sudo ip link del osvbng-mgmt-br 2>/dev/null || true
        ;;
    logs)
        docker compose logs -f osvbng
        ;;
    *)
        echo "Usage: $0 {start|stop|logs}"
        exit 1
        ;;
esac
