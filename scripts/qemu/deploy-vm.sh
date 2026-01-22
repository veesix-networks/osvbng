#!/bin/bash
set -e

if [ ! -t 0 ]; then
    exec < /dev/tty
fi

VM_NAME="osvbng"
VM_MEMORY=4096
VM_VCPUS=4
QCOW2_URL="https://github.com/veesix-networks/osvbng/releases/latest/download/osvbng-debian12.qcow2.gz"
INSTALL_DIR="/var/lib/libvirt/images"
MGMT_BRIDGE="br-mgmt"

declare -a ACCESS_INTERFACES
declare -a CORE_INTERFACES

# PCI NIC detection functions
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

detect_tui_tool() {
    if command -v whiptail &> /dev/null; then
        TUI_TOOL="whiptail"
    elif command -v dialog &> /dev/null; then
        TUI_TOOL="dialog"
    else
        echo "Error: Neither whiptail nor dialog found."
        echo "Please install one of them:"
        echo "  Debian/Ubuntu: sudo apt install whiptail"
        echo "  RHEL/CentOS:   sudo yum install newt"
        echo "  Or:            sudo apt install dialog"
        exit 1
    fi
}

show_banner() {
    $TUI_TOOL --title "osvbng KVM Deployment" --msgbox \
"This script will deploy osvbng as a KVM virtual machine with PCI passthrough for DPDK.

Requirements:
- KVM/libvirt installed
- bridge-utils installed (for management bridge)
- IOMMU enabled in BIOS and kernel
- At least 2 network interfaces for PCI passthrough
- 4GB RAM and 4 vCPUs recommended

Press OK to continue." 17 70
}

check_requirements() {
    if ! command -v virsh &> /dev/null; then
        $TUI_TOOL --title "Error" --msgbox "libvirt/virsh not found. Please install libvirt first." 8 60
        exit 1
    fi

    if ! command -v virt-install &> /dev/null; then
        $TUI_TOOL --title "Error" --msgbox "virt-install not found. Please install it first:\n\n  Debian/Ubuntu: apt install virtinst\n  RHEL/CentOS: yum install virt-install" 10 65
        exit 1
    fi

    if ! command -v curl &> /dev/null; then
        $TUI_TOOL --title "Error" --msgbox "curl not found. Please install it first." 8 60
        exit 1
    fi

    if ! command -v gunzip &> /dev/null; then
        $TUI_TOOL --title "Error" --msgbox "gunzip not found. Please install gzip first." 8 60
        exit 1
    fi

    if ! check_iommu_enabled; then
        $TUI_TOOL --title "Warning" --msgbox \
"IOMMU does not appear to be enabled.

For Intel CPUs, add 'intel_iommu=on' to kernel parameters.
For AMD CPUs, add 'amd_iommu=on' to kernel parameters.

Edit /etc/default/grub and add to GRUB_CMDLINE_LINUX, then run:
  sudo update-grub && sudo reboot

Continue anyway?" 14 70
    fi

    if ! check_vfio_loaded; then
        $TUI_TOOL --title "Info" --msgbox \
"vfio-pci module not loaded. This script will attempt to load it.

If this fails, you may need to:
  echo 'vfio-pci' | sudo tee /etc/modules-load.d/vfio-pci.conf
  sudo modprobe vfio-pci" 12 70

        modprobe vfio-pci 2>/dev/null || true
    fi
}

select_pci_devices() {
    declare -a checklist_items
    local nic_count=0

    while IFS= read -r lspci_line; do
        local slot=$(echo "$lspci_line" | cut -d' ' -f1)
        local sysfs_path="/sys/bus/pci/devices/0000:$slot"

        local model=$(echo "$lspci_line" | sed 's/^[^ ]* //; s/Ethernet controller: //')
        model=$(echo "$model" | sed 's/ \[[0-9a-f:]*\]//g; s/ (rev [0-9a-f]*)//g')
        model="${model:0:45}"

        local netif=""
        [ -d "$sysfs_path/net" ] && netif=$(ls "$sysfs_path/net" 2>/dev/null | head -1)

        local label="$model"
        [ -n "$netif" ] && label="$model ($netif)"

        checklist_items+=("$slot" "$label" "OFF")
        nic_count=$((nic_count + 1))
    done < <(lspci | grep -i 'Ethernet controller')

    if [ $nic_count -eq 0 ]; then
        $TUI_TOOL --title "Error" --msgbox "No PCI network devices found." 8 60
        exit 1
    fi

    local access_result
    access_result=$($TUI_TOOL --title "Select Access Interface(s)" --checklist \
"Select PCI device(s) for ACCESS (customer-facing). SPACE=select, ENTER=confirm." 20 78 $nic_count \
"${checklist_items[@]}" 3>&1 1>&2 2>&3)

    if [ -z "$access_result" ]; then
        echo "No access interface selected."
        exit 1
    fi

    ACCESS_INTERFACES=()
    for item in $access_result; do
        item="${item//\"/}"
        [ -n "$item" ] && ACCESS_INTERFACES+=("$item")
    done

    declare -a core_items
    local core_count=0

    while IFS= read -r lspci_line; do
        local slot=$(echo "$lspci_line" | cut -d' ' -f1)
        local sysfs_path="/sys/bus/pci/devices/0000:$slot"

        local already_used=false
        for used in "${ACCESS_INTERFACES[@]}"; do
            [ "$slot" = "$used" ] && already_used=true && break
        done
        [ "$already_used" = true ] && continue

        local model=$(echo "$lspci_line" | sed 's/^[^ ]* //; s/Ethernet controller: //')
        model=$(echo "$model" | sed 's/ \[[0-9a-f:]*\]//g; s/ (rev [0-9a-f]*)//g')
        model="${model:0:45}"

        local netif=""
        [ -d "$sysfs_path/net" ] && netif=$(ls "$sysfs_path/net" 2>/dev/null | head -1)

        local label="$model"
        [ -n "$netif" ] && label="$model ($netif)"

        core_items+=("$slot" "$label" "OFF")
        core_count=$((core_count + 1))
    done < <(lspci | grep -i 'Ethernet controller')

    if [ $core_count -eq 0 ]; then
        $TUI_TOOL --title "Error" --msgbox "No interfaces remaining for CORE selection." 8 60
        exit 1
    fi

    local core_result
    core_result=$($TUI_TOOL --title "Select Core Interface(s)" --checklist \
"Select PCI device(s) for CORE (network-facing). SPACE=select, ENTER=confirm." 20 78 $core_count \
"${core_items[@]}" 3>&1 1>&2 2>&3)

    if [ -z "$core_result" ]; then
        echo "No core interface selected."
        exit 1
    fi

    CORE_INTERFACES=()
    for item in $core_result; do
        item="${item//\"/}"
        [ -n "$item" ] && CORE_INTERFACES+=("$item")
    done
}

configure_vm_settings() {
    VM_NAME=$($TUI_TOOL --inputbox "Enter VM name:" 8 60 "$VM_NAME" 3>&1 1>&2 2>&3)
    VM_MEMORY=$($TUI_TOOL --inputbox "Enter memory (MB):" 8 60 "$VM_MEMORY" 3>&1 1>&2 2>&3)
    VM_VCPUS=$($TUI_TOOL --inputbox "Enter number of vCPUs:" 8 60 "$VM_VCPUS" 3>&1 1>&2 2>&3)

    while true; do
        MGMT_BRIDGE=$($TUI_TOOL --inputbox "Enter management bridge name:" 10 70 "$MGMT_BRIDGE" 3>&1 1>&2 2>&3)

        if brctl show "$MGMT_BRIDGE" &>/dev/null; then
            break
        else
            if $TUI_TOOL --title "Bridge Not Found" --yesno \
"Bridge '$MGMT_BRIDGE' does not exist.

Do you want to continue anyway? (VM creation may fail)" 10 60; then
                break
            fi
        fi
    done
}

download_image() {
    local dest="$INSTALL_DIR/${VM_NAME}.qcow2"
    local dest_gz="${dest}.gz"

    if [ -f "$dest" ]; then
        if ! $TUI_TOOL --title "Image Exists" --yesno \
"Image already exists at $dest. Download again?" 8 60; then
            QCOW2_PATH="$dest"
            return
        fi
        rm -f "$dest"
    fi

    if [ -f "$dest_gz" ]; then
        $TUI_TOOL --title "Compressed Image Found" --infobox "Found existing $dest_gz, extracting..." 8 60
    else
        $TUI_TOOL --title "Downloading" --infobox "Downloading osvbng image (~1GB)..." 8 60

        if ! curl -fL -o "$dest_gz" "$QCOW2_URL"; then
            $TUI_TOOL --title "Error" --msgbox "Failed to download image from $QCOW2_URL" 8 60
            exit 1
        fi
    fi

    $TUI_TOOL --title "Extracting" --infobox "Extracting image..." 8 60

    if ! gunzip -f "$dest_gz"; then
        $TUI_TOOL --title "Error" --msgbox "Failed to extract image" 8 60
        exit 1
    fi

    QCOW2_PATH="$dest"
}

# Convert short PCI address (BB:SS.F) to virt-install format (pci_0000_BB_SS_F)
pci_to_hostdev() {
    local addr=$1
    if [[ "$addr" != *:*:* ]]; then
        addr="0000:$addr"
    fi
    echo "pci_$(echo "$addr" | tr ':.' '_')"
}

create_vm() {
    if virsh list --all | grep -q "$VM_NAME"; then
        if $TUI_TOOL --title "VM Exists" --yesno \
"VM '$VM_NAME' already exists. Destroy and recreate?" 8 60; then
            virsh destroy "$VM_NAME" 2>/dev/null || true
            virsh undefine "$VM_NAME" 2>/dev/null || true
        else
            exit 1
        fi
    fi

    local cmd="virt-install"
    cmd="$cmd --name $VM_NAME"
    cmd="$cmd --memory $VM_MEMORY"
    cmd="$cmd --vcpus $VM_VCPUS"
    cmd="$cmd --cpu host-passthrough"
    cmd="$cmd --machine q35"
    cmd="$cmd --os-variant linux2022"
    cmd="$cmd --import"
    cmd="$cmd --disk path=$QCOW2_PATH,format=qcow2,bus=virtio"
    cmd="$cmd --network bridge=$MGMT_BRIDGE,model=virtio"
    cmd="$cmd --graphics none"
    cmd="$cmd --console pty,target_type=serial"
    cmd="$cmd --memorybacking hugepages=yes"
    cmd="$cmd --noautoconsole"

    for pci_addr in "${ACCESS_INTERFACES[@]}"; do
        local hostdev=$(pci_to_hostdev "$pci_addr")
        cmd="$cmd --hostdev $hostdev"
    done

    for pci_addr in "${CORE_INTERFACES[@]}"; do
        local hostdev=$(pci_to_hostdev "$pci_addr")
        cmd="$cmd --hostdev $hostdev"
    done

    local xml_file="/tmp/${VM_NAME}.xml"
    if ! $cmd --print-xml > "$xml_file"; then
        $TUI_TOOL --title "Error" --msgbox "Failed to generate VM XML." 8 60
        exit 1
    fi

    if ! virsh define "$xml_file"; then
        $TUI_TOOL --title "Error" --msgbox "Failed to define VM. Check $xml_file for errors." 8 60
        exit 1
    fi

    echo "VM XML saved to: $xml_file"
}

configure_hugepages() {
    local nr_hugepages=$((VM_MEMORY / 2))

    if [ ! -d "/dev/hugepages" ]; then
        mkdir -p /dev/hugepages
        mount -t hugetlbfs hugetlbfs /dev/hugepages
    fi

    echo $nr_hugepages > /sys/kernel/mm/hugepages/hugepages-2048kB/nr_hugepages

    if ! grep -q "hugetlbfs" /etc/fstab; then
        echo "hugetlbfs /dev/hugepages hugetlbfs defaults 0 0" >> /etc/fstab
    fi
}

show_summary() {
    local access_list=$(printf '%s\n' "${ACCESS_INTERFACES[@]}" | tr '\n' ', ' | sed 's/,$//')
    local core_list=$(printf '%s\n' "${CORE_INTERFACES[@]}" | tr '\n' ', ' | sed 's/,$//')

    $TUI_TOOL --title "Deployment Complete" --msgbox \
"osvbng VM has been created successfully!

VM Name: $VM_NAME
Memory: $VM_MEMORY MB
vCPUs: $VM_VCPUS
Management Bridge: $MGMT_BRIDGE (virtio eth0)
Access Interface(s): $access_list (${#ACCESS_INTERFACES[@]} total)
Core Interface(s): $core_list (${#CORE_INTERFACES[@]} total)

Start the VM with:
  sudo virsh start $VM_NAME

Connect to console:
  sudo virsh console $VM_NAME

Access CLI:
  sudo virsh console $VM_NAME

  Default Login: root / osvbng
  # Then inside VM: osvbngcli" 24 78
}

main() {
    if [ "$EUID" -ne 0 ]; then
        echo "This script must be run as root"
        exit 1
    fi

    detect_tui_tool
    show_banner
    check_requirements
    select_pci_devices
    configure_vm_settings
    download_image
    configure_hugepages
    create_vm
    show_summary
}

main