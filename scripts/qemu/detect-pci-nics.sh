#!/bin/bash

detect_pci_nics() {
    local -n result=$1

    while IFS= read -r line; do
        pci_addr=$(echo "$line" | awk '{print $1}')
        description=$(echo "$line" | cut -d' ' -f2-)

        iommu_group=$(basename "$(readlink /sys/bus/pci/devices/$pci_addr/iommu_group)" 2>/dev/null)
        if [ -z "$iommu_group" ]; then
            iommu_group="N/A"
        fi

        driver=$(basename "$(readlink /sys/bus/pci/devices/$pci_addr/driver)" 2>/dev/null)
        if [ -z "$driver" ]; then
            driver="none"
        fi

        result+=("$pci_addr|$description|IOMMU:$iommu_group|Driver:$driver")
    done < <(lspci -nn | grep -i 'ethernet\|network')
}

check_iommu_enabled() {
    if ! dmesg | grep -q "IOMMU enabled"; then
        if [ ! -d "/sys/kernel/iommu_groups" ] || [ -z "$(ls -A /sys/kernel/iommu_groups 2>/dev/null)" ]; then
            return 1
        fi
    fi
    return 0
}

check_vfio_loaded() {
    lsmod | grep -q vfio_pci
}

get_nic_info() {
    local pci_addr=$1
    local info=""

    if [ -d "/sys/bus/pci/devices/$pci_addr/net" ]; then
        local netdev=$(ls "/sys/bus/pci/devices/$pci_addr/net" 2>/dev/null | head -n1)
        if [ -n "$netdev" ]; then
            info="Interface: $netdev"

            local mac=$(cat "/sys/class/net/$netdev/address" 2>/dev/null)
            if [ -n "$mac" ]; then
                info="$info, MAC: $mac"
            fi

            local state=$(cat "/sys/class/net/$netdev/operstate" 2>/dev/null)
            if [ -n "$state" ]; then
                info="$info, State: $state"
            fi
        fi
    fi

    echo "$info"
}
