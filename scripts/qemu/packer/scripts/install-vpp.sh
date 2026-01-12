#!/bin/bash
set -e

export DEBIAN_FRONTEND=noninteractive
export VPP_INSTALL_SKIP_SYSCTL=true

if [ -z "$VPP_VERSION" ]; then
    echo "Error: VPP_VERSION environment variable must be set"
    exit 1
fi

curl -s https://packagecloud.io/install/repositories/fdio/release/script.deb.sh | bash

apt-get update && apt-get install -y --no-install-recommends \
    numactl \
    vpp=${VPP_VERSION} \
    vpp-plugin-core=${VPP_VERSION} \
    vpp-plugin-dpdk=${VPP_VERSION} \
    libvppinfra=${VPP_VERSION} \
    && rm -rf /var/lib/apt/lists/*

systemctl disable vpp
systemctl stop vpp || true
