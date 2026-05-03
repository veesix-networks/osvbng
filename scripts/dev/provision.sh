#!/bin/bash
# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Dev VM provisioning script — runs inside the VM.
# Installs all dependencies needed to build and test osvbng from source.
# Idempotent — safe to re-run when versions.env changes.

set -euo pipefail

export DEBIAN_FRONTEND=noninteractive

GOLANG_VERSION="${GOLANG_VERSION:-1.24}"
DATAPLANE_VERSION="${DATAPLANE_VERSION:-25.10-release}"

# --- Base packages ---

apt-get update
apt-get install -y \
    build-essential \
    pkg-config \
    libnl-3-dev \
    libnl-route-3-dev \
    libmnl-dev \
    curl \
    wget \
    git \
    sudo \
    vim \
    htop \
    tcpdump \
    lsb-release \
    gnupg \
    ca-certificates \
    sqlite3 \
    libsqlite3-dev \
    protobuf-compiler \
    rsync

# --- Go ---

if [ ! -d "/usr/local/go" ] || ! /usr/local/go/bin/go version 2>/dev/null | grep -q "go${GOLANG_VERSION}"; then
    rm -rf /usr/local/go
    curl -fsSL "https://go.dev/dl/go${GOLANG_VERSION}.linux-amd64.tar.gz" | tar -C /usr/local -xz
fi

cat > /etc/profile.d/go.sh <<'EOF'
export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"
EOF

export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"

GOBIN=/usr/local/bin go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
GOBIN=/usr/local/bin go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# --- VPP ---

export VPP_INSTALL_SKIP_SYSCTL=true

if ! dpkg -s vpp &>/dev/null; then
    curl -s https://packagecloud.io/install/repositories/fdio/release/script.deb.sh | bash

    apt-get update
    apt-get install -y --no-install-recommends \
        numactl \
        "vpp=${DATAPLANE_VERSION}" \
        "vpp-plugin-core=${DATAPLANE_VERSION}" \
        "vpp-plugin-dpdk=${DATAPLANE_VERSION}" \
        "libvppinfra=${DATAPLANE_VERSION}"

    systemctl disable vpp
    systemctl stop vpp || true
fi

# --- FRR ---

if ! dpkg -s frr &>/dev/null; then
    curl -s https://deb.frrouting.org/frr/keys.gpg | tee /usr/share/keyrings/frrouting.gpg > /dev/null
    echo "deb [signed-by=/usr/share/keyrings/frrouting.gpg] https://deb.frrouting.org/frr $(lsb_release -s -c) frr-stable" \
        | tee /etc/apt/sources.list.d/frr.list

    apt-get update
    apt-get install -y --no-install-recommends \
        frr \
        frr-pythontools

    systemctl disable frr
    systemctl stop frr || true
fi

# --- Docker ---

if ! command -v docker &>/dev/null; then
    install -m 0755 -d /etc/apt/keyrings
    curl -fsSL https://download.docker.com/linux/debian/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
    chmod a+r /etc/apt/keyrings/docker.gpg

    echo \
        "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/debian \
        $(lsb_release -s -c) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null

    apt-get update
    apt-get install -y \
        docker-ce \
        docker-ce-cli \
        containerd.io \
        docker-buildx-plugin \
        docker-compose-plugin

    systemctl enable docker
fi

# --- Containerlab ---

CONTAINERLAB_VERSION="${CONTAINERLAB_VERSION:-0.73.0}"

if ! command -v containerlab &>/dev/null; then
    bash -c "$(curl -sL https://get.containerlab.dev)" -- -v "${CONTAINERLAB_VERSION}"
fi

# --- QEMU/KVM + Packer (for building osvbng QEMU images) ---

apt-get update
apt-get install -y \
    qemu-system-x86 \
    qemu-utils \
    cloud-image-utils

if ! command -v packer &>/dev/null; then
    curl -fsSL https://apt.releases.hashicorp.com/gpg | gpg --dearmor -o /usr/share/keyrings/hashicorp.gpg
    echo "deb [signed-by=/usr/share/keyrings/hashicorp.gpg] https://apt.releases.hashicorp.com $(lsb_release -s -c) main" \
        | tee /etc/apt/sources.list.d/hashicorp.list
    apt-get update
    apt-get install -y packer
fi

if [ -w /dev/kvm ]; then
    echo "  KVM:    available (nested virtualization working)"
else
    echo "  WARNING: /dev/kvm not available. Packer builds will use TCG (slow)."
    echo "  Enable nested virt on host: modprobe kvm_intel nested=1"
fi

# --- Kernel modules (VRF, MPLS) ---

cat > /etc/modules-load.d/osvbng.conf <<EOF
vrf
mpls_router
mpls_iptunnel
dummy
EOF

cat > /etc/sysctl.d/99-mpls.conf <<EOF
net.mpls.platform_labels=1048575
EOF

for mod in vrf mpls_router mpls_iptunnel dummy; do
    modprobe "$mod" 2>/dev/null || true
done
sysctl -p /etc/sysctl.d/99-mpls.conf 2>/dev/null || true

# --- Hugepages mount ---

mkdir -p /dev/hugepages
if ! mountpoint -q /dev/hugepages; then
    mount -t hugetlbfs -o pagesize=2M none /dev/hugepages
fi
grep -q hugetlbfs /etc/fstab || echo "none /dev/hugepages hugetlbfs pagesize=2M 0 0" >> /etc/fstab

# --- Dev user setup ---

if id dev &>/dev/null; then
    usermod -aG docker dev 2>/dev/null || true
fi

# --- Cleanup ---

apt-get clean
rm -rf /var/lib/apt/lists/*

echo "Provisioning complete."
echo "  Go:     $(/usr/local/go/bin/go version)"
echo "  VPP:    $(dpkg -s vpp 2>/dev/null | grep Version | awk '{print $2}')"
echo "  FRR:    $(dpkg -s frr 2>/dev/null | grep Version | awk '{print $2}')"
echo "  Docker: $(docker --version 2>/dev/null)"
