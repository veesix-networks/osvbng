#!/bin/bash
set -e

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

cat > /etc/systemd/system/vpp.service.d/override.conf <<'EOF'
[Unit]
Requires=osvbng-config.service
After=osvbng-config.service

[Service]
ExecStart=
ExecStart=/usr/bin/vpp -c /etc/osvbng/dataplane.conf
EOF

cat > /etc/systemd/system/osvbng.service <<'EOF'
[Unit]
Description=OSVBNG Daemon
After=network.target vpp.service
Requires=vpp.service

[Service]
Type=simple
User=root
Group=osvbng
ExecStart=/usr/local/bin/osvbngd -config /etc/osvbng/osvbng.yaml
Restart=on-failure
RestartSec=5
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
