#!/bin/bash
set -e

CONTAINER_NAME="${1}"
shift

if [ -z "$CONTAINER_NAME" ] || [ $# -lt 1 ]; then
    echo "Usage: $0 <container_name> <container_if1[:host_if1]> [container_if2[:host_if2]] ..."
    echo ""
    echo "Examples:"
    echo "  Production with physical NICs (eth0=mgmt, eth1=access, eth2=core):"
    echo "    $0 osvbng eth0:br-mgmt eth1:enp0s3 eth2:enp1s3"
    echo ""
    echo "  Production with bonded interfaces:"
    echo "    $0 osvbng eth0:br-mgmt eth1:bond0 eth2:bond1"
    echo ""
    echo "  Testing without bridging (mgmt + access only):"
    echo "    $0 osvbng eth0 eth1"
    echo ""
    echo "This creates veth pairs and optionally bridges them to host interfaces."
    exit 1
fi

if ! docker inspect "$CONTAINER_NAME" >/dev/null 2>&1; then
    echo "Error: Container '$CONTAINER_NAME' not found"
    exit 1
fi

CONTAINER_PID=$(docker inspect -f '{{.State.Pid}}' "$CONTAINER_NAME")
if [ -z "$CONTAINER_PID" ] || [ "$CONTAINER_PID" = "0" ]; then
    echo "Error: Container '$CONTAINER_NAME' is not running"
    exit 1
fi

echo "Setting up network namespace link for container '$CONTAINER_NAME'..."
sudo mkdir -p /var/run/netns
sudo ln -sf /proc/$CONTAINER_PID/ns/net "/var/run/netns/$CONTAINER_NAME"

for MAPPING in "$@"; do
    IFS=':' read -r CONTAINER_IF HOST_IF <<< "$MAPPING"

    if [ -z "$CONTAINER_IF" ]; then
        echo "Error: Invalid mapping '$MAPPING'"
        continue
    fi

    VETH_NAME="veth-${CONTAINER_IF}"

    echo ""
    echo "Creating veth pair: $VETH_NAME (host) <-> $CONTAINER_IF (container)"

    if ip link show "$VETH_NAME" >/dev/null 2>&1; then
        echo "Removing existing interface $VETH_NAME..."
        sudo ip link delete "$VETH_NAME"
    fi

    sudo ip link add "$VETH_NAME" type veth peer "$CONTAINER_IF" netns "$CONTAINER_NAME"
    sudo ip link set "$VETH_NAME" up
    sudo ip netns exec "$CONTAINER_NAME" ip link set "$CONTAINER_IF" up

    echo "Created $VETH_NAME <-> $CONTAINER_IF"

    if [ -n "$HOST_IF" ]; then
        if ! ip link show "$HOST_IF" >/dev/null 2>&1; then
            echo "Warning: Host interface '$HOST_IF' not found, skipping bridge setup"
            continue
        fi

        BRIDGE_NAME="br-${CONTAINER_IF}"

        echo "Creating bridge $BRIDGE_NAME..."

        if ip link show "$BRIDGE_NAME" >/dev/null 2>&1; then
            echo "Bridge $BRIDGE_NAME already exists, reusing..."
        else
            sudo ip link add "$BRIDGE_NAME" type bridge
            sudo ip link set "$BRIDGE_NAME" up
        fi

        echo "Attaching $VETH_NAME to $BRIDGE_NAME..."
        sudo ip link set "$VETH_NAME" master "$BRIDGE_NAME"

        echo "Attaching $HOST_IF to $BRIDGE_NAME..."
        sudo ip link set "$HOST_IF" master "$BRIDGE_NAME"
        sudo ip link set "$HOST_IF" up

        echo "Bridged: $HOST_IF <-> $BRIDGE_NAME <-> $VETH_NAME <-> $CONTAINER_IF"
    fi
done

echo ""
echo "Interface setup complete"
echo ""
echo "Container interfaces:"
sudo ip netns exec "$CONTAINER_NAME" ip link show | grep -E "^[0-9]+:" | awk '{print "  " $2}' | sed 's/:$//'

echo ""
echo "Monitor container logs:"
echo "  docker logs -f $CONTAINER_NAME"
