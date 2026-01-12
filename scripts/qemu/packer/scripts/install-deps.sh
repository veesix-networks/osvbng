#!/bin/bash
set -e

export DEBIAN_FRONTEND=noninteractive

apt-get update && apt-get install -y \
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
    cloud-init \
    linux-headers-$(uname -r)

echo "hugetlbfs /dev/hugepages hugetlbfs defaults 0 0" >> /etc/fstab
mkdir -p /dev/hugepages

echo 'GRUB_CMDLINE_LINUX="$GRUB_CMDLINE_LINUX default_hugepagesz=2M hugepagesz=2M hugepages=1024 iommu=pt intel_iommu=on"' >> /etc/default/grub
update-grub
