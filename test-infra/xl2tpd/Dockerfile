# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

FROM debian:12-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
        xl2tpd ppp iproute2 iputils-ping tcpdump procps rsyslog && \
    rm -rf /var/lib/apt/lists/*

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
