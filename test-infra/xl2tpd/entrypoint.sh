#!/bin/sh
set -e

for _ in $(seq 1 60); do
    if ip link show eth1 >/dev/null 2>&1; then
        break
    fi
    sleep 1
done

ip link set eth1 up
if ! ip -4 addr show eth1 | grep -q 'inet '; then
    ip addr add 10.10.0.2/30 dev eth1
fi
ip route replace default via 10.10.0.1

for _ in $(seq 1 120); do
    if ping -c 1 -W 1 10.20.0.2 >/dev/null 2>&1; then
        break
    fi
    sleep 1
done

mkdir -p /var/run/xl2tpd
rm -f /var/run/xl2tpd/xl2tpd.pid

rsyslogd -n &
sleep 1
tail -F /var/log/syslog &

xl2tpd -D -c /etc/xl2tpd/xl2tpd.conf -s /etc/xl2tpd/l2tp-secrets &
PID=$!

sleep 2
echo "c lns" > /var/run/xl2tpd/l2tp-control

wait $PID
