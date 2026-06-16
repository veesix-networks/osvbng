#!/bin/bash
set -e

cat > /etc/sysctl.d/99-osvbng.conf <<'EOF'
net.unix.max_dgram_qlen = 10000

net.core.rmem_max = 67108864
net.core.wmem_max = 67108864
net.core.rmem_default = 1048576
net.core.wmem_default = 1048576
EOF

sysctl -p /etc/sysctl.d/99-osvbng.conf || true

cat > /etc/systemd/system/osvbng-config.service <<'EOF'
[Unit]
Description=Generate OSVBNG external configurations
Before=vpp.service frr.service

[Service]
Type=oneshot
ExecStart=/usr/local/bin/osvbngd -config /etc/osvbng/osvbng.yaml generate-external
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
EOF

mkdir -p /etc/systemd/system/vpp.service.d

# Type=simple reports "active" the moment vpp exec's, but vpp's
# /run/osvbng/dataplane_api.sock takes ~2s more to bind. ExecStartPost
# blocks unit activation until the socket exists so systemctl
# is-active actually means "API listener is up" and downstream callers
# (osvbngd, upgrade runner) no longer need a separate socket-presence
# wait.
cat > /etc/systemd/system/vpp.service.d/override.conf <<'EOF'
[Unit]
Requires=osvbng-config.service
After=osvbng-config.service

[Service]
ExecStartPre=-/sbin/ip netns add dataplane
ExecStartPre=/sbin/ip netns exec dataplane ip link set lo up
ExecStart=
ExecStart=/usr/bin/vpp -c /etc/osvbng/dataplane.conf
ExecStartPost=/bin/sh -c 'for i in $(seq 1 120); do [ -S /run/osvbng/dataplane_api.sock ] && exit 0; sleep 0.5; done; echo "vpp api socket /run/osvbng/dataplane_api.sock did not appear within 60s" >&2; exit 1'
EOF

# Shadow the frr package unit with one that omits Before=network.target.
# Package /lib/systemd/system/frr.service marks frr as a "network
# provider" (Wants=network.target + Before=network.target). Combined
# with our After=vpp.service ordering and vpp's After=network.target,
# that's a cycle (vpp ← After ← network.target ← Before ← frr ← After
# ← vpp). systemd's cycle-breaker deletes network.target/start, leaving
# vpp.service unable to start at boot. A drop-in `Before=` empty
# doesn't reliably clear the base list on systemd 252, so we replace
# the unit entirely. vpp owns the dataplane; frr rides on top of it,
# not the other way around.
cat > /etc/systemd/system/frr.service <<'EOF'
[Unit]
Description=FRRouting
Documentation=https://frrouting.readthedocs.io/en/latest/setup.html
After=network-pre.target systemd-sysctl.service vpp.service osvbng-config.service
Requires=vpp.service
PartOf=vpp.service
OnFailure=heartbeat-failed@%n

[Service]
Nice=-5
Type=forking
NotifyAccess=all
StartLimitInterval=3m
StartLimitBurst=3
TimeoutSec=2m
WatchdogSec=60s
RestartSec=5
Restart=always
LimitNOFILE=2048
PIDFile=/var/run/frr/watchfrr.pid
NetworkNamespacePath=/var/run/netns/dataplane
ExecStart=/usr/lib/frr/frrinit.sh start
ExecStop=/usr/lib/frr/frrinit.sh stop
ExecReload=/usr/lib/frr/frrinit.sh reload

[Install]
WantedBy=multi-user.target
EOF
# Remove any prior drop-in path so the new unit is the only source of truth.
rm -f /etc/systemd/system/frr.service.d/dataplane-netns.conf
rmdir /etc/systemd/system/frr.service.d 2>/dev/null || true

cat > /etc/systemd/system/osvbng.service <<'EOF'
[Unit]
Description=OSVBNG Daemon
After=network.target vpp.service
Requires=vpp.service

[Service]
Type=simple
User=root
Group=osvbng
RuntimeDirectory=osvbng
RuntimeDirectoryMode=0750
RuntimeDirectoryPreserve=yes
ExecStart=/usr/local/bin/osvbngd -config /etc/osvbng/osvbng.yaml
Restart=always
RestartSec=2
StartLimitIntervalSec=0
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable osvbng-config
systemctl enable vpp
systemctl enable frr
systemctl enable osvbng
systemctl enable serial-getty@ttyS0.service
