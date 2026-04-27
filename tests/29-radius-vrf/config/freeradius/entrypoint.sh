#!/bin/bash
# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

set -e

# Wait for the bng1<->freeradius link. eth1's IPv4 is configured from
# outside (Robot suite setup uses nsenter; this image lacks iproute2).
for _ in $(seq 1 60); do
    [ -e /sys/class/net/eth1 ] && break
    sleep 1
done

exec freeradius -X
