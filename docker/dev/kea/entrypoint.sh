#!/bin/sh
mkdir -p /run/kea /var/lib/kea

echo "Waiting for eth0..."
while ! ip link show eth0 >/dev/null 2>&1; do
    sleep 1
done
sleep 2

echo "Starting Kea DHCPv4 and DHCPv6 servers..."
kea-dhcp4 -c /etc/kea/kea-dhcp4.conf &
kea-dhcp6 -c /etc/kea/kea-dhcp6.conf &
wait
