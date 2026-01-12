#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/detect-pci-nics.sh"

VM_NAME="osvbng"
VM_MEMORY=4096
VM_VCPUS=4
QCOW2_URL="https://github.com/veesix-networks/osvbng/releases/latest/download/osvbng-debian12.qcow2"
INSTALL_DIR="/var/lib/libvirt/images"

show_banner() {
    whiptail --title "osvbng KVM Deployment" --msgbox \
"This script will deploy osvbng as a KVM virtual machine with PCI passthrough for DPDK.

Requirements:
- KVM/libvirt installed
- IOMMU enabled in BIOS and kernel
- At least 2 network interfaces for PCI passthrough
- 4GB RAM and 4 vCPUs recommended

Press OK to continue." 16 70
}

check_requirements() {
    if ! command -v virsh &> /dev/null; then
        whiptail --title "Error" --msgbox "libvirt/virsh not found. Please install libvirt first." 8 60
        exit 1
    fi

    if ! check_iommu_enabled; then
        whiptail --title "Warning" --msgbox \
"IOMMU does not appear to be enabled.

For Intel CPUs, add 'intel_iommu=on' to kernel parameters.
For AMD CPUs, add 'amd_iommu=on' to kernel parameters.

Edit /etc/default/grub and add to GRUB_CMDLINE_LINUX, then run:
  sudo update-grub && sudo reboot

Continue anyway?" 14 70
    fi

    if ! check_vfio_loaded; then
        whiptail --title "Info" --msgbox \
"vfio-pci module not loaded. This script will attempt to load it.

If this fails, you may need to:
  echo 'vfio-pci' | sudo tee /etc/modules-load.d/vfio-pci.conf
  sudo modprobe vfio-pci" 12 70

        modprobe vfio-pci 2>/dev/null || true
    fi
}

select_pci_devices() {
    declare -a nics
    detect_pci_nics nics

    if [ ${#nics[@]} -eq 0 ]; then
        whiptail --title "Error" --msgbox "No PCI network devices found." 8 60
        exit 1
    fi

    declare -a menu_items
    for nic in "${nics[@]}"; do
        pci_addr=$(echo "$nic" | cut -d'|' -f1)
        description=$(echo "$nic" | cut -d'|' -f2-)

        nic_info=$(get_nic_info "$pci_addr")
        if [ -n "$nic_info" ]; then
            description="$description ($nic_info)"
        fi

        menu_items+=("$pci_addr" "$description")
    done

    ACCESS_INTERFACE=$(whiptail --title "Select Access Interface" --menu \
"Select the PCI device for the ACCESS interface (customer-facing):" 20 78 10 \
"${menu_items[@]}" 3>&1 1>&2 2>&3)

    if [ -z "$ACCESS_INTERFACE" ]; then
        echo "No access interface selected."
        exit 1
    fi

    CORE_INTERFACE=$(whiptail --title "Select Core Interface" --menu \
"Select the PCI device for the CORE interface (network-facing):" 20 78 10 \
"${menu_items[@]}" 3>&1 1>&2 2>&3)

    if [ -z "$CORE_INTERFACE" ]; then
        echo "No core interface selected."
        exit 1
    fi

    if [ "$ACCESS_INTERFACE" = "$CORE_INTERFACE" ]; then
        whiptail --title "Error" --msgbox "Access and Core interfaces must be different." 8 60
        exit 1
    fi
}

configure_vm_settings() {
    VM_NAME=$(whiptail --inputbox "Enter VM name:" 8 60 "$VM_NAME" 3>&1 1>&2 2>&3)
    VM_MEMORY=$(whiptail --inputbox "Enter memory (MB):" 8 60 "$VM_MEMORY" 3>&1 1>&2 2>&3)
    VM_VCPUS=$(whiptail --inputbox "Enter number of vCPUs:" 8 60 "$VM_VCPUS" 3>&1 1>&2 2>&3)
}

download_image() {
    local dest="$INSTALL_DIR/${VM_NAME}.qcow2"

    if [ -f "$dest" ]; then
        if ! whiptail --title "Image Exists" --yesno \
"Image already exists at $dest. Download again?" 8 60; then
            QCOW2_PATH="$dest"
            return
        fi
    fi

    whiptail --title "Downloading" --infobox "Downloading osvbng image..." 8 60

    if ! wget -O "$dest" "$QCOW2_URL"; then
        whiptail --title "Error" --msgbox "Failed to download image from $QCOW2_URL" 8 60
        exit 1
    fi

    QCOW2_PATH="$dest"
}

generate_vm_xml() {
    local xml_file="/tmp/${VM_NAME}.xml"

    cat > "$xml_file" <<EOF
<domain type='kvm'>
  <name>$VM_NAME</name>
  <memory unit='MiB'>$VM_MEMORY</memory>
  <vcpu placement='static'>$VM_VCPUS</vcpu>
  <cpu mode='host-passthrough'>
    <topology sockets='1' cores='$VM_VCPUS' threads='1'/>
    <numa>
      <cell id='0' cpus='0-$((VM_VCPUS-1))' memory='$VM_MEMORY' unit='MiB' memAccess='shared'/>
    </numa>
  </cpu>
  <os>
    <type arch='x86_64' machine='pc-q35'>hvm</type>
    <boot dev='hd'/>
  </os>
  <features>
    <acpi/>
    <apic/>
    <pae/>
  </features>
  <clock offset='utc'/>
  <on_poweroff>destroy</on_poweroff>
  <on_reboot>restart</on_reboot>
  <on_crash>destroy</on_crash>
  <devices>
    <emulator>/usr/bin/kvm</emulator>
    <disk type='file' device='disk'>
      <driver name='qemu' type='qcow2'/>
      <source file='$QCOW2_PATH'/>
      <target dev='vda' bus='virtio'/>
    </disk>
    <interface type='network'>
      <source network='default'/>
      <model type='virtio'/>
    </interface>
    <hostdev mode='subsystem' type='pci' managed='yes'>
      <source>
        <address domain='0x${ACCESS_INTERFACE%%:*}' bus='0x${ACCESS_INTERFACE#*:}' slot='0x${ACCESS_INTERFACE##*:}' function='0x0'/>
      </source>
    </hostdev>
    <hostdev mode='subsystem' type='pci' managed='yes'>
      <source>
        <address domain='0x${CORE_INTERFACE%%:*}' bus='0x${CORE_INTERFACE#*:}' slot='0x${CORE_INTERFACE##*:}' function='0x0'/>
      </source>
    </hostdev>
    <serial type='pty'>
      <target type='isa-serial' port='0'>
        <model name='isa-serial'/>
      </target>
    </serial>
    <console type='pty'>
      <target type='serial' port='0'/>
    </console>
    <memballoon model='virtio'/>
  </devices>
  <memoryBacking>
    <hugepages/>
  </memoryBacking>
</domain>
EOF

    echo "$xml_file"
}

create_vm() {
    local xml_file=$(generate_vm_xml)

    if virsh list --all | grep -q "$VM_NAME"; then
        if whiptail --title "VM Exists" --yesno \
"VM '$VM_NAME' already exists. Destroy and recreate?" 8 60; then
            virsh destroy "$VM_NAME" 2>/dev/null || true
            virsh undefine "$VM_NAME" 2>/dev/null || true
        else
            exit 1
        fi
    fi

    if ! virsh define "$xml_file"; then
        whiptail --title "Error" --msgbox "Failed to create VM. Check $xml_file for errors." 8 60
        exit 1
    fi

    rm -f "$xml_file"
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
    whiptail --title "Deployment Complete" --msgbox \
"osvbng VM has been created successfully!

VM Name: $VM_NAME
Memory: $VM_MEMORY MB
vCPUs: $VM_VCPUS
Access Interface: $ACCESS_INTERFACE
Core Interface: $CORE_INTERFACE

Start the VM with:
  virsh start $VM_NAME

Connect to console:
  virsh console $VM_NAME

Access CLI:
  virsh console $VM_NAME
  # Then inside VM: osvbngcli" 20 70
}

main() {
    if [ "$EUID" -ne 0 ]; then
        echo "This script must be run as root"
        exit 1
    fi

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
