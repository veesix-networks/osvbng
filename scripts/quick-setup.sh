#!/bin/bash
#
# osvbng Quick Setup Script
# Usage: curl -sL https://v6n.io/osvbng | sudo bash
#

set -e

OSVBNG_SKIP_DEPS="${OSVBNG_SKIP_DEPS:-false}"

function check_root() {
    if [ "$EUID" -ne 0 ]; then
        echo "Error: This script must be run as root or with sudo"
        exit 1
    fi
}

function check_os() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        if [ "$ID" != "debian" ] && [ "$ID" != "ubuntu" ]; then
            echo "Error: Unsupported OS: $ID (Supported: Debian, Ubuntu)"
            exit 1
        fi
    else
        echo "Error: Cannot determine the operating system"
        exit 1
    fi
}

function install_deps() {
    if [ "${OSVBNG_SKIP_DEPS}" = "true" ]; then
        echo "Skipping dependency installation (OSVBNG_SKIP_DEPS=true)"
        return
    fi

    echo "Installing dependencies..."
    apt-get update -y
    apt-get install -y libvirt-daemon-system qemu-kvm virtinst curl whiptail gzip

    systemctl enable libvirtd
    systemctl start libvirtd

    echo "Dependencies installed successfully"
}

function download_and_run() {
    local deploy_script_url="https://raw.githubusercontent.com/veesix-networks/osvbng/dev/scripts/qemu/deploy-vm.sh"
    local deploy_script_path="/tmp/osvbng-deploy-vm.sh"

    echo "Downloading deployment script..."

    if ! curl -fsSL -o "$deploy_script_path" "$deploy_script_url"; then
        echo "Error: Failed to download deployment script"
        exit 1
    fi

    chmod +x "$deploy_script_path"

    echo ""
    echo "Starting osvbng deployment..."
    echo ""

    bash "$deploy_script_path"
}

function main() {
    cat << 'EOF'
                 _
  ___  _____   _| |__  _ __   __ _
 / _ \/ __\ \ / / '_ \| '_ \ / _` |
| (_) \__ \\ V /| |_) | | | | (_| |
 \___/|___/ \_/ |_.__/|_| |_|\__, |
                             |___/

EOF

    check_root
    check_os
    install_deps
    download_and_run
}

main "$@"
