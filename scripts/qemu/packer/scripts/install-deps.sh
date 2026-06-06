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

sed -i 's/GRUB_CMDLINE_LINUX_DEFAULT="\([^"]*\)"/GRUB_CMDLINE_LINUX_DEFAULT="\1 console=ttyS0,115200 default_hugepagesz=2M hugepagesz=2M hugepages=1024 iommu=pt intel_iommu=on net.ifnames=0 biosdevname=0"/' /etc/default/grub
update-grub

# Pin the management NIC to eth0. cloud-init left to its own devices
# writes /etc/netplan/50-cloud-init.yaml with set-name: ens3, plus
# generates a /run/udev/rules.d/99-netplan-*.rules that wins over GRUB's
# net.ifnames=0. Disable cloud-init network regeneration and ship a
# static netplan so eth0 is what enumerates on every boot.
mkdir -p /etc/cloud/cloud.cfg.d
cat > /etc/cloud/cloud.cfg.d/99-disable-network-config.cfg <<'EOF'
network: {config: disabled}
EOF

rm -f /etc/netplan/50-cloud-init.yaml

mkdir -p /etc/netplan
cat > /etc/netplan/01-osvbng-mgmt.yaml <<'EOF'
network:
  version: 2
  renderer: networkd
  ethernets:
    eth0:
      dhcp4: true
EOF
chmod 0600 /etc/netplan/01-osvbng-mgmt.yaml
