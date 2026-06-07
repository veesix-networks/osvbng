# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

FROM debian:bookworm

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update -qy \
 && apt-get install -y --no-install-recommends \
        bridge-utils \
        iproute2 \
        python3 \
        python3-ipy \
        socat \
        qemu-kvm \
        qemu-utils \
        xorriso \
        openssh-client \
        ca-certificates \
 && rm -rf /var/lib/apt/lists/*

ARG IMAGE
COPY $IMAGE /osvbng.qcow2
COPY *.py /
RUN chmod 0755 /launch.py /healthcheck.py

EXPOSE 22 8080 9090

HEALTHCHECK --interval=10s --timeout=5s --start-period=180s --retries=12 \
    CMD ["/healthcheck.py"]
ENTRYPOINT ["/launch.py"]
