#!/bin/bash
set -e

cat > /etc/systemd/system/osvbng.service <<'EOF'
[Unit]
Description=OSVBNG Daemon
After=network.target vpp.service
Requires=vpp.service

[Service]
Type=simple
User=root
Group=osvbng
ExecStart=/usr/local/bin/osvbngd -c /etc/osvbng/osvbng.yaml
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl disable vpp
systemctl disable frr
systemctl disable osvbng
