#!/bin/bash
# Copyright 2025 Veesix Networks Ltd
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

set -euo pipefail

cat > /etc/modules-load.d/osvbng-test.conf <<EOF
vrf
mpls_router
mpls_iptunnel
dummy
EOF

cat > /etc/sysctl.d/99-mpls.conf <<EOF
net.mpls.platform_labels=1048575
EOF
