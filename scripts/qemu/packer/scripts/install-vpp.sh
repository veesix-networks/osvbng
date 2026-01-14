#!/bin/bash
set -e

export DEBIAN_FRONTEND=noninteractive
export VPP_INSTALL_SKIP_SYSCTL=true

if [ -z "$DATAPLANE_VERSION" ]; then
    echo "Error: DATAPLANE_VERSION environment variable must be set"
    exit 1
fi

curl -s https://packagecloud.io/install/repositories/fdio/release/script.deb.sh | bash

apt-get update && apt-get install -y --no-install-recommends \
    numactl \
    vpp=${DATAPLANE_VERSION} \
    vpp-plugin-core=${DATAPLANE_VERSION} \
    vpp-plugin-dpdk=${DATAPLANE_VERSION} \
    libvppinfra=${DATAPLANE_VERSION} \
    && rm -rf /var/lib/apt/lists/*

systemctl disable vpp
systemctl stop vpp || true
