#!/bin/bash
# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Deploy osvbng as a KVM/libvirt VM with:
#   - Host kernel cmdline tuned for VPP (1G hugepages, isolcpus, AMD-Vi pt mode)
#   - SMT-aware vCPU pinning
#   - Per-NUMA hugepage backing
#   - PCI passthrough for dataplane NICs (libvirt-managed device detach)
#   - Guest sees correct NUMA topology for passthrough NICs via pci-expander-bus
#
# Interactive (TUI) by default. Pass flags for non-interactive use.

set -eu

if [ ! -t 0 ]; then
    exec < /dev/tty
fi

# ---------------------------------------------------------------------------
# Defaults / constants
# ---------------------------------------------------------------------------
VM_NAME="osvbng"
VM_IMAGE_PATH=""
INSTALL_DIR="/var/lib/libvirt/images"
QCOW2_URL="https://github.com/veesix-networks/osvbng/releases/latest/download/osvbng-debian12.qcow2.gz"
MGMT_BRIDGE="br-mgmt"

NUMA_NODES=""
HOST_CORES=""
VM_CORES=""
PASSTHROUGH_PCI=""

APPLY_HOST=0
DEFINE_VM=0
SWITCH_POWERSAVE=0
INTERACTIVE=1
REBOOT_NEEDED=0

STATE_DIR="/etc/osvbng"
GRUB_DROPIN="/etc/default/grub.d/98-osvbng.cfg"
SYSCTL_DROPIN="/etc/sysctl.d/98-osvbng.conf"
IRQBALANCE_CONF="/etc/default/irqbalance"
HUGETLB_CONF="$STATE_DIR/hugepages.conf"
LIB_DIR="/usr/local/lib/osvbng"
HUGETLB_RESERVE_SH="$LIB_DIR/hugetlb-reserve.sh"
DISABLE_THP_SH="$LIB_DIR/disable-thp.sh"
HUGETLB_SVC="/etc/systemd/system/osvbng-hugetlb-reserve.service"
THP_SVC="/etc/systemd/system/osvbng-disable-thp.service"
VFIO_MODULES_LOAD="/etc/modules-load.d/osvbng-vfio.conf"

declare -a PASSTHROUGH_INTERFACES
declare -a VM_NUMA_NODES
declare -a NUMA_HUGEPAGES_GB
declare -a VCPU_PINS
declare -A CPU_NODE
declare -A CPU_SIBLING
declare -a ALL_NUMA_NODES

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
usage() {
    cat <<EOF
Usage: $(basename "$0") [options]

Options:
  -p PATH    VM image full path (default: \$INSTALL_DIR/<name>.qcow2)
  -n NAME    VM name (default: osvbng)
  -e NODES   NUMA nodes to allocate VM cores from (e.g. "0" or "0,1")
  -c CORES   Host cores kept un-isolated (e.g. "0,1,64,65"); other cores on
             the selected NUMA nodes are isolated and given to the VM
  -v CORES   VM cores (explicit; overrides the -c derivation)
  -m GB      Memory in GB, comma-separated per VM NUMA node ("32" or "32,32")
  -P PCI     NIC PCI BDFs to pass through to osvbng, comma-separated
  -b NAME    Management bridge name (default: br-mgmt)
  -a         Apply host configuration (GRUB / sysctl / hugepage svc / vfio modules)
  -d         Define VM (virsh define + autostart)
  -s         Switch governor to powersave (only effective with BIOS Turbo off)
  -h         Help

With no options, runs in interactive TUI mode.
EOF
}

require_root() {
    if [ "$EUID" -ne 0 ]; then
        echo "Run as root." >&2
        exit 1
    fi
}

detect_tui_tool() {
    if command -v whiptail &> /dev/null; then
        TUI_TOOL="whiptail"
    elif command -v dialog &> /dev/null; then
        TUI_TOOL="dialog"
    else
        echo "Install whiptail (or dialog)." >&2
        exit 1
    fi
}

write_if_changed() {
    # write_if_changed <path> <content>
    # Returns 0 if file was written (changed/created), 1 if unchanged.
    local path=$1 content=$2 tmp
    tmp=$(mktemp)
    printf '%s' "$content" > "$tmp"
    if [ -f "$path" ] && cmp -s "$tmp" "$path"; then
        rm -f "$tmp"
        return 1
    fi
    install -D -m 0644 "$tmp" "$path"
    rm -f "$tmp"
    return 0
}

expand_range() {
    # expand_range "2-5,7,9-11" -> "2 3 4 5 7 9 10 11"
    local range=$1 out=()
    local IFS=','
    for part in $range; do
        if [[ $part == *-* ]]; then
            local s=${part%-*} e=${part#*-}
            local i
            for ((i=s; i<=e; i++)); do out+=("$i"); done
        else
            [ -n "$part" ] && out+=("$part")
        fi
    done
    echo "${out[@]:-}"
}

compress_range() {
    # compress_range "2 3 4 5 7 9 10 11" -> "2-5,7,9-11"
    local sorted out="" start="" prev="" c
    sorted=$(printf '%s\n' "$@" | sort -n -u)
    for c in $sorted; do
        if [ -z "$start" ]; then
            start=$c
            prev=$c
        elif [ "$c" -eq $((prev + 1)) ]; then
            prev=$c
        else
            if [ "$start" -eq "$prev" ]; then
                out="$out,$start"
            else
                out="$out,$start-$prev"
            fi
            start=$c
            prev=$c
        fi
    done
    if [ -n "$start" ]; then
        if [ "$start" -eq "$prev" ]; then
            out="$out,$start"
        else
            out="$out,$start-$prev"
        fi
    fi
    echo "${out#,}"
}

pci_to_hostdev() {
    local addr=$1
    [[ $addr != *:*:* ]] && addr="0000:$addr"
    echo "pci_$(echo "$addr" | tr ':.' '_')"
}

# ---------------------------------------------------------------------------
# Topology discovery
# ---------------------------------------------------------------------------
build_topology() {
    local cpu_dir cpu node sibs_file s
    ALL_NUMA_NODES=()
    while IFS= read -r d; do
        ALL_NUMA_NODES+=("${d##*/node}")
    done < <(ls -d /sys/devices/system/node/node[0-9]* 2>/dev/null | sort -V)

    for cpu_dir in /sys/devices/system/cpu/cpu[0-9]*; do
        cpu=${cpu_dir##*/cpu}
        node=$(ls -d "$cpu_dir"/node[0-9]* 2>/dev/null | head -1 | sed 's|.*/node||')
        [ -z "$node" ] && node=0
        CPU_NODE[$cpu]=$node

        sibs_file="$cpu_dir/topology/thread_siblings_list"
        if [ -f "$sibs_file" ]; then
            local sibs
            sibs=$(<"$sibs_file")
            CPU_SIBLING[$cpu]=""
            local IFS=','
            for s in $sibs; do
                if [[ $s == *-* ]]; then
                    local a=${s%-*} b=${s#*-} i
                    for ((i=a; i<=b; i++)); do
                        [ "$i" != "$cpu" ] && CPU_SIBLING[$cpu]=$i && break
                    done
                else
                    [ "$s" != "$cpu" ] && CPU_SIBLING[$cpu]=$s && break
                fi
            done
        fi
    done
}

pci_numa_node() {
    local addr=$1 n
    [[ $addr != *:*:* ]] && addr="0000:$addr"
    n=$(cat "/sys/bus/pci/devices/$addr/numa_node" 2>/dev/null || echo 0)
    [ "$n" -lt 0 ] && n=0
    echo "$n"
}

# ---------------------------------------------------------------------------
# Core selection
# ---------------------------------------------------------------------------
derive_vm_cores_from_host_reserve() {
    # Given HOST_CORES + NUMA_NODES, derive VM_CORES as: all CPUs on the
    # selected NUMA nodes except host-reserved ones. Both SMT threads of each
    # physical core are given to the VM together unless one is in HOST_CORES.
    local -A is_host=()
    local c
    for c in $(expand_range "$HOST_CORES"); do
        is_host[$c]=1
    done

    local -a vm_cpus=()
    local node want
    local IFS_save=$IFS
    IFS=$'\n'
    for node in $(printf '%s\n' "$(expand_range "$NUMA_NODES")"); do
        IFS=$IFS_save
        for want in "${!CPU_NODE[@]}"; do
            [ "${CPU_NODE[$want]}" = "$node" ] || continue
            [ -n "${is_host[$want]:-}" ] && continue
            local sib=${CPU_SIBLING[$want]:-}
            if [ -n "$sib" ] && [ -n "${is_host[$sib]:-}" ]; then
                continue
            fi
            vm_cpus+=("$want")
        done
        IFS=$'\n'
    done
    IFS=$IFS_save
    VM_CORES=$(compress_range "${vm_cpus[@]}")
}

auto_pick_host_cores() {
    # Reserve 2 physical cores (+ siblings) per VM NUMA node for the host,
    # plus everything on NUMA nodes not used by the VM.
    local -a host_cpus=()
    local node cpu sib
    local -A in_vm_nodes=()
    for node in $(expand_range "$NUMA_NODES"); do
        in_vm_nodes[$node]=1
    done

    # Whole NUMA nodes not used by VM -> host
    for node in "${ALL_NUMA_NODES[@]}"; do
        [ -n "${in_vm_nodes[$node]:-}" ] && continue
        for cpu in "${!CPU_NODE[@]}"; do
            [ "${CPU_NODE[$cpu]}" = "$node" ] && host_cpus+=("$cpu")
        done
    done

    # Two physical cores per VM NUMA node for host
    local -A taken=()
    for node in $(expand_range "$NUMA_NODES"); do
        local taken_count=0
        local -a node_cpus_sorted
        node_cpus_sorted=$(for cpu in "${!CPU_NODE[@]}"; do
            [ "${CPU_NODE[$cpu]}" = "$node" ] && echo "$cpu"
        done | sort -n)
        for cpu in $node_cpus_sorted; do
            [ -n "${taken[$cpu]:-}" ] && continue
            host_cpus+=("$cpu")
            taken[$cpu]=1
            sib=${CPU_SIBLING[$cpu]:-}
            if [ -n "$sib" ]; then
                host_cpus+=("$sib")
                taken[$sib]=1
            fi
            taken_count=$((taken_count + 1))
            [ $taken_count -ge 2 ] && break
        done
    done

    HOST_CORES=$(compress_range "${host_cpus[@]}")
}

build_vcpu_pin_list() {
    # Emit the cpuset order vCPU 0..N-1 will map to. We bias vCPU 0 to the
    # lowest-numbered physical core in the VM set so VPP main/osvbngd land on
    # a predictable core; SMT siblings appear later (higher vCPU indices).
    local cpus_in
    cpus_in=$(expand_range "$VM_CORES")
    if [ -z "$cpus_in" ]; then
        echo "Failed to derive VM core list." >&2
        exit 1
    fi

    local -A is_vm=()
    local c
    for c in $cpus_in; do is_vm[$c]=1; done

    local -a primaries=()
    local -a secondaries=()
    for c in $cpus_in; do
        local sib=${CPU_SIBLING[$c]:-}
        if [ -z "$sib" ] || [ -z "${is_vm[$sib]:-}" ]; then
            primaries+=("$c")
        elif [ "$c" -lt "$sib" ]; then
            primaries+=("$c")
        else
            secondaries+=("$c")
        fi
    done

    VCPU_PINS=()
    for c in $(printf '%s\n' "${primaries[@]}" | sort -n); do VCPU_PINS+=("$c"); done
    for c in $(printf '%s\n' "${secondaries[@]}" | sort -n); do VCPU_PINS+=("$c"); done
}

# ---------------------------------------------------------------------------
# Host configuration writers (all idempotent)
# ---------------------------------------------------------------------------
write_grub_dropin() {
    local cmdline
    cmdline="amd_iommu=on iommu=pt"
    cmdline="$cmdline default_hugepagesz=1G hugepagesz=1G"
    cmdline="$cmdline isolcpus=$VM_CORES nohz_full=$VM_CORES rcu_nocbs=$VM_CORES"
    cmdline="$cmdline transparent_hugepage=never"

    local content
    content=$(cat <<EOF
# Managed by osvbng/scripts/qemu/deploy-vm.sh — edits will be overwritten.
GRUB_CMDLINE_LINUX_DEFAULT="\${GRUB_CMDLINE_LINUX_DEFAULT} $cmdline"
EOF
)
    if write_if_changed "$GRUB_DROPIN" "$content"; then
        echo "Updated $GRUB_DROPIN"
        if command -v update-grub >/dev/null 2>&1; then
            update-grub
        else
            grub-mkconfig -o /boot/grub/grub.cfg
        fi
        REBOOT_NEEDED=1
    fi
}

write_sysctl_dropin() {
    local content
    content="# Managed by osvbng deploy-vm.sh
kernel.numa_balancing=0
kernel.sched_rt_runtime_us=-1
"
    if write_if_changed "$SYSCTL_DROPIN" "$content"; then
        sysctl -p "$SYSCTL_DROPIN" >/dev/null 2>&1 || true
    fi
}

write_hugepage_service() {
    mkdir -p "$STATE_DIR" "$LIB_DIR"

    # KEY=value config: sourceable by bash, no positional parsing.
    # Format: HUGEPAGES_1G_NODE_<id>=<count>
    local conf="# osvbng — 1G hugepage reservation per NUMA node.
# Managed by deploy-vm.sh. Sourced by hugetlb-reserve.sh.
"
    local id=0 node
    for node in "${VM_NUMA_NODES[@]}"; do
        conf="${conf}HUGEPAGES_1G_NODE_${node}=${NUMA_HUGEPAGES_GB[$id]:-0}
"
        id=$((id + 1))
    done
    write_if_changed "$HUGETLB_CONF" "$conf" || true

    local sh
    sh='#!/bin/bash
# osvbng — pre-mount 1G hugepage reservation, per NUMA node.
# Driven by /etc/osvbng/hugepages.conf (KEY=value).
set -euo pipefail

CONFIG=/etc/osvbng/hugepages.conf
[[ -r $CONFIG ]] || exit 0

# shellcheck disable=SC1090
source "$CONFIG"

shopt -s nullglob

for var in $(compgen -A variable | grep "^HUGEPAGES_1G_NODE_"); do
    node=${var#HUGEPAGES_1G_NODE_}
    count=${!var}
    [[ -z $count || $count == 0 ]] && continue
    sysfs=/sys/devices/system/node/node${node}/hugepages/hugepages-1048576kB/nr_hugepages
    if [[ -w $sysfs ]]; then
        printf "%s\n" "$count" > "$sysfs"
        printf "osvbng: reserved %s 1G page(s) on NUMA %s\n" "$count" "$node"
    fi
done
'
    if write_if_changed "$HUGETLB_RESERVE_SH" "$sh"; then
        chmod 0755 "$HUGETLB_RESERVE_SH"
    fi

    local svc
    svc='[Unit]
Description=osvbng pre-boot 1G hugepage allocator
Documentation=https://github.com/veesix-networks/osvbng
DefaultDependencies=no
After=local-fs.target
Before=dev-hugepages.mount
Before=libvirtd.service
ConditionPathExists=/sys/kernel/mm/hugepages
ConditionPathExists=/etc/osvbng/hugepages.conf

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/usr/local/lib/osvbng/hugetlb-reserve.sh
StandardOutput=journal+console

[Install]
WantedBy=sysinit.target
'
    if write_if_changed "$HUGETLB_SVC" "$svc"; then
        systemctl daemon-reload
        systemctl enable osvbng-hugetlb-reserve.service >/dev/null 2>&1 || true
    fi
    # systemd ships dev-hugepages.mount as a static unit. Make sure it's
    # actually wanted by local-fs.target so it fires on boot.
    systemctl enable dev-hugepages.mount >/dev/null 2>&1 || true
}

write_thp_service() {
    mkdir -p "$LIB_DIR"

    local sh
    sh='#!/bin/bash
# osvbng — quiet transparent hugepages at boot. THP merging of 4K pages
# competes with VPP'\''s explicit 1G hugepage backing and adds khugepaged
# work to the shared control-plane core.
set -e

for f in \
    /sys/kernel/mm/transparent_hugepage/enabled \
    /sys/kernel/mm/transparent_hugepage/defrag
do
    [[ -w $f ]] && printf "never\n" > "$f"
done
'
    if write_if_changed "$DISABLE_THP_SH" "$sh"; then
        chmod 0755 "$DISABLE_THP_SH"
    fi

    local svc
    svc='[Unit]
Description=osvbng disable transparent hugepages
DefaultDependencies=no
After=local-fs.target
Before=osvbng-hugetlb-reserve.service
ConditionPathExists=/sys/kernel/mm/transparent_hugepage

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/usr/local/lib/osvbng/disable-thp.sh

[Install]
WantedBy=sysinit.target
'
    if write_if_changed "$THP_SVC" "$svc"; then
        systemctl daemon-reload
        systemctl enable osvbng-disable-thp.service >/dev/null 2>&1 || true
    fi
    [ -x "$DISABLE_THP_SH" ] && "$DISABLE_THP_SH" 2>/dev/null || true
}

write_vfio_modules() {
    local content
    content="# Managed by osvbng deploy-vm.sh
vfio
vfio_iommu_type1
vfio_pci
"
    write_if_changed "$VFIO_MODULES_LOAD" "$content" || true
}

configure_irqbalance() {
    [ -f "$IRQBALANCE_CONF" ] || return 0

    local total_cpus
    total_cpus=$(nproc)
    local groups=$(( (total_cpus + 31) / 32 ))
    local -a mask
    local i
    for ((i=0; i<groups; i++)); do mask[$i]=$((0xffffffff)); done
    for cpu in $(expand_range "$VM_CORES"); do
        local g=$((groups - 1 - cpu / 32))
        local b=$((cpu % 32))
        mask[$g]=$(( mask[g] & ~(1 << b) ))
    done
    local joined=""
    for ((i=0; i<groups; i++)); do
        joined="$joined,$(printf '%x' "${mask[$i]}")"
    done
    joined=${joined#,}

    if grep -q "^IRQBALANCE_BANNED_CPUS=" "$IRQBALANCE_CONF"; then
        sed -i "s/^IRQBALANCE_BANNED_CPUS=.*/IRQBALANCE_BANNED_CPUS=$joined/" "$IRQBALANCE_CONF"
    else
        echo "IRQBALANCE_BANNED_CPUS=$joined" >> "$IRQBALANCE_CONF"
    fi
    if grep -q "^IRQBALANCE_ONESHOT=" "$IRQBALANCE_CONF"; then
        sed -i "s/^IRQBALANCE_ONESHOT=.*/IRQBALANCE_ONESHOT=yes/" "$IRQBALANCE_CONF"
    else
        echo "IRQBALANCE_ONESHOT=yes" >> "$IRQBALANCE_CONF"
    fi
    systemctl try-restart irqbalance >/dev/null 2>&1 || true
}

switch_governor_powersave() {
    [ "$SWITCH_POWERSAVE" -eq 1 ] || return 0
    [ -d /sys/devices/system/cpu/cpu0/cpufreq ] || return 0
    local cpu
    for cpu in /sys/devices/system/cpu/cpu[0-9]*/cpufreq/scaling_governor; do
        [ -w "$cpu" ] && echo powersave > "$cpu" 2>/dev/null || true
    done
}

disable_libvirt_apparmor() {
    # On Debian, libvirt generates a per-VM AppArmor profile that blocks
    # qemu from reading /sys/devices/system/node/* — needed for NUMA-aware
    # memory placement. A dedicated osvbng host runs only this VM, so the
    # per-VM mediation buys little and breaks NUMA pinning. Disable it.
    local conf=/etc/libvirt/qemu.conf
    [ -f "$conf" ] || return 0
    if grep -qE '^[[:space:]]*security_driver[[:space:]]*=' "$conf"; then
        sed -i 's|^[[:space:]]*security_driver[[:space:]]*=.*|security_driver = "none"|' "$conf"
    elif grep -qE '^[[:space:]]*#[[:space:]]*security_driver[[:space:]]*=' "$conf"; then
        sed -i 's|^[[:space:]]*#[[:space:]]*security_driver[[:space:]]*=.*|security_driver = "none"|' "$conf"
    else
        printf '\n# Managed by osvbng deploy-vm.sh — required for NUMA pinning\nsecurity_driver = "none"\n' >> "$conf"
    fi
}

apply_host_config() {
    write_grub_dropin
    write_sysctl_dropin
    write_hugepage_service
    write_thp_service
    write_vfio_modules
    configure_irqbalance
    switch_governor_powersave
    disable_libvirt_apparmor

    # Activate everything now so re-running the script doesn't require a
    # reboot unless the kernel cmdline itself changed.
    systemctl start osvbng-disable-thp.service       >/dev/null 2>&1 || true
    systemctl restart osvbng-hugetlb-reserve.service >/dev/null 2>&1 || true
    systemctl start dev-hugepages.mount              >/dev/null 2>&1 || true
    systemctl restart libvirtd.service               >/dev/null 2>&1 || true
}

# ---------------------------------------------------------------------------
# Pre-flight verification before VM definition
# ---------------------------------------------------------------------------
verify_iommu_groups_clean() {
    local pci grp_path others
    for pci in "${PASSTHROUGH_INTERFACES[@]}"; do
        [ -z "$pci" ] && continue
        [[ $pci != *:*:* ]] && pci="0000:$pci"
        grp_path="/sys/bus/pci/devices/$pci/iommu_group/devices"
        if [ ! -d "$grp_path" ]; then
            echo "WARNING: $pci has no IOMMU group (IOMMU not enabled or no driver)." >&2
            continue
        fi
        others=$(ls "$grp_path" | grep -v -E "^${pci//./\\.}$" || true)
        if [ -n "$others" ]; then
            local extra=""
            local d
            for d in $others; do
                # tolerate other passthrough devices we're already taking
                local match=0 other
                for other in "${PASSTHROUGH_INTERFACES[@]}"; do
                    [[ $other != *:*:* ]] && other="0000:$other"
                    [ "$d" = "$other" ] && match=1 && break
                done
                [ "$match" -eq 0 ] && extra="$extra $d"
            done
            if [ -n "$extra" ]; then
                echo "WARNING: $pci shares its IOMMU group with:$extra" >&2
            fi
        fi
    done
}

verify_host_prepared() {
    local fail=0
    if ! grep -qE 'iommu=pt' /proc/cmdline; then
        echo "WARNING: /proc/cmdline missing iommu=pt — host may need reboot after -a." >&2
        fail=1
    fi
    if ! grep -qE 'hugepagesz=1G' /proc/cmdline; then
        echo "WARNING: /proc/cmdline missing hugepagesz=1G — host may need reboot after -a." >&2
        fail=1
    fi
    local id=0 node need have
    for node in "${VM_NUMA_NODES[@]}"; do
        need=${NUMA_HUGEPAGES_GB[$id]:-0}
        have=$(cat /sys/devices/system/node/node$node/hugepages/hugepages-1048576kB/free_hugepages 2>/dev/null || echo 0)
        if [ "$have" -lt "$need" ]; then
            echo "WARNING: NUMA $node has $have free 1G hugepages, VM wants $need." >&2
            fail=1
        fi
        id=$((id + 1))
    done
    return $fail
}

# ---------------------------------------------------------------------------
# Interactive flow (TUI)
# ---------------------------------------------------------------------------
show_banner() {
    $TUI_TOOL --title "osvbng KVM Deployment" --msgbox \
"Deploys osvbng as a KVM virtual machine with PCI passthrough for DPDK and
host kernel/cgroup/hugepage tuning for VPP.

Requirements:
  - KVM/libvirt installed
  - bridge-utils or iproute2 (management bridge already created)
  - AMD-Vi / Intel VT-d enabled in BIOS
  - At least 1 PCI NIC to pass through
  - 8+ GB RAM and 4+ vCPUs

Press OK to continue." 18 72
}

check_requirements() {
    local missing=""
    command -v virsh >/dev/null || missing="$missing virsh"
    command -v virt-install >/dev/null || missing="$missing virt-install"
    command -v curl >/dev/null || missing="$missing curl"
    command -v gunzip >/dev/null || missing="$missing gunzip"
    command -v ip >/dev/null || missing="$missing ip"
    if [ -n "$missing" ]; then
        $TUI_TOOL --title "Missing dependencies" --msgbox \
"These tools are required but not found:$missing

On Debian/Ubuntu:
  apt install libvirt-daemon-system virtinst curl gzip iproute2" 12 70
        exit 1
    fi

    if [ ! -d /sys/kernel/iommu_groups ] || [ -z "$(ls -A /sys/kernel/iommu_groups 2>/dev/null)" ]; then
        $TUI_TOOL --title "IOMMU not enabled" --yesno \
"IOMMU is not active. Enable AMD-Vi / VT-d in BIOS, then re-run with -a to add
'amd_iommu=on iommu=pt' (or 'intel_iommu=on') to the kernel cmdline.

Continue anyway?" 12 70 || exit 1
    fi

    lsmod | grep -q vfio_pci || modprobe vfio-pci 2>/dev/null || true
}

select_pci_devices() {
    declare -a items
    local nic_count=0 lspci_line slot model netif label numa

    while IFS= read -r lspci_line; do
        slot=$(echo "$lspci_line" | cut -d' ' -f1)
        model=$(echo "$lspci_line" | sed 's/^[^ ]* //; s/Ethernet controller: //')
        model=$(echo "$model" | sed 's/ \[[0-9a-f:]*\]//g; s/ (rev [0-9a-f]*)//g')
        model="${model:0:40}"
        netif=""
        [ -d "/sys/bus/pci/devices/0000:$slot/net" ] && netif=$(ls "/sys/bus/pci/devices/0000:$slot/net" 2>/dev/null | head -1)
        numa=$(cat "/sys/bus/pci/devices/0000:$slot/numa_node" 2>/dev/null || echo 0)
        [ "$numa" -lt 0 ] && numa=0
        label="N${numa} $model"
        [ -n "$netif" ] && label="$label ($netif)"
        items+=("$slot" "$label" "OFF")
        nic_count=$((nic_count + 1))
    done < <(lspci | grep -i 'Ethernet controller')

    if [ $nic_count -eq 0 ]; then
        $TUI_TOOL --title "Error" --msgbox "No PCI Ethernet devices found." 8 60
        exit 1
    fi

    local sel
    sel=$($TUI_TOOL --title "Pass-through NIC(s)" --checklist \
"Select PCI device(s) to pass through to osvbng.
SPACE selects, ENTER confirms." 20 78 $nic_count "${items[@]}" 3>&1 1>&2 2>&3) || exit 1
    PASSTHROUGH_INTERFACES=()
    for item in $sel; do
        item="${item//\"/}"
        [ -n "$item" ] && PASSTHROUGH_INTERFACES+=("$item")
    done

    if [ ${#PASSTHROUGH_INTERFACES[@]} -eq 0 ]; then
        $TUI_TOOL --title "Error" --msgbox "No interfaces selected." 8 60
        exit 1
    fi
}

prompt_vm_settings() {
    VM_NAME=$($TUI_TOOL --inputbox "VM name:" 8 60 "$VM_NAME" 3>&1 1>&2 2>&3) || exit 1

    # Determine VM NUMA nodes from selected NICs
    local -A nodes_seen=()
    local pci numa
    for pci in "${PASSTHROUGH_INTERFACES[@]}"; do
        numa=$(pci_numa_node "$pci")
        nodes_seen[$numa]=1
    done
    VM_NUMA_NODES=()
    for numa in "${!nodes_seen[@]}"; do VM_NUMA_NODES+=("$numa"); done
    if [ ${#VM_NUMA_NODES[@]} -eq 0 ]; then VM_NUMA_NODES=(0); fi
    NUMA_NODES=$(IFS=,; echo "${VM_NUMA_NODES[*]}")

    # Memory per NUMA node
    local mem_input default_mem="32"
    [ ${#VM_NUMA_NODES[@]} -gt 1 ] && default_mem=$(IFS=,; printf '32,%.0s' "${VM_NUMA_NODES[@]}"; echo) && default_mem=${default_mem%,}
    mem_input=$($TUI_TOOL --inputbox \
"Memory in GB (1G hugepages), comma-separated per VM NUMA node.
Selected nodes: $NUMA_NODES" 12 70 "$default_mem" 3>&1 1>&2 2>&3) || exit 1
    IFS=',' read -ra NUMA_HUGEPAGES_GB <<<"$mem_input"

    # Total vCPU count
    local default_vcpus=16 vcpus
    vcpus=$($TUI_TOOL --inputbox \
"Total VM vCPU count (SMT siblings of the chosen physical cores are
allocated automatically; pick a multiple of NUMA-node count)." 12 70 "$default_vcpus" 3>&1 1>&2 2>&3) || exit 1

    # Auto host/vm split: reserve 2 physical cores per VM NUMA node for host
    auto_pick_host_cores
    derive_vm_cores_from_host_reserve

    # Trim VM_CORES down to requested vcpu count if user asked for less
    local current
    current=$(expand_range "$VM_CORES" | wc -w)
    if [ "$vcpus" -lt "$current" ]; then
        local -a kept=() c
        local i=0
        for c in $(expand_range "$VM_CORES"); do
            [ $i -ge "$vcpus" ] && break
            kept+=("$c")
            i=$((i + 1))
        done
        VM_CORES=$(compress_range "${kept[@]}")
    fi

    while true; do
        MGMT_BRIDGE=$($TUI_TOOL --inputbox "Management bridge name:" 10 70 "$MGMT_BRIDGE" 3>&1 1>&2 2>&3) || exit 1
        if ip link show type bridge dev "$MGMT_BRIDGE" >/dev/null 2>&1; then
            break
        fi
        $TUI_TOOL --title "Bridge not found" --yesno \
"Bridge '$MGMT_BRIDGE' does not exist on this host.
Continue anyway? (VM start will fail until the bridge exists.)" 10 70 && break
    done

    if $TUI_TOOL --title "Apply host config?" --yesno \
"Apply host kernel/sysctl/hugepage configuration?

- GRUB cmdline: 1G hugepages, isolcpus, amd_iommu=on iommu=pt
- 1G hugepage reservation systemd service (per-NUMA)
- transparent_hugepage=never
- sysctls: kernel.numa_balancing=0, kernel.sched_rt_runtime_us=-1
- vfio modules-load
- irqbalance ban-list

Choosing No means you've already applied these (or will manually)." 18 72; then
        APPLY_HOST=1
    fi

    if $TUI_TOOL --title "Define VM?" --yesno \
"Define the VM in libvirt and set autostart now?" 9 70; then
        DEFINE_VM=1
    fi
}

# ---------------------------------------------------------------------------
# VM image
# ---------------------------------------------------------------------------
download_image() {
    [ -n "$VM_IMAGE_PATH" ] || VM_IMAGE_PATH="$INSTALL_DIR/${VM_NAME}.qcow2"
    local dest=$VM_IMAGE_PATH
    local dest_gz="${dest}.gz"

    if [ -f "$dest" ]; then
        if [ $INTERACTIVE -eq 1 ]; then
            $TUI_TOOL --title "Image exists" --yesno \
"Image already exists at $dest. Re-download?" 8 70 || return 0
        else
            return 0
        fi
        rm -f "$dest"
    fi

    mkdir -p "$(dirname "$dest")"
    if [ ! -f "$dest_gz" ]; then
        [ $INTERACTIVE -eq 1 ] && $TUI_TOOL --title "Downloading" --infobox "Downloading osvbng image..." 8 60
        curl -fL --progress-bar -o "$dest_gz" "$QCOW2_URL"
    fi
    [ $INTERACTIVE -eq 1 ] && $TUI_TOOL --title "Extracting" --infobox "Extracting image..." 8 60
    gunzip -f "$dest_gz"
}

# ---------------------------------------------------------------------------
# VM definition
# ---------------------------------------------------------------------------
build_virtinstall_args() {
    local total_vcpus
    total_vcpus=$(expand_range "$VM_CORES" | wc -w)

    local sockets=${#VM_NUMA_NODES[@]}
    local cores_per_socket=$((total_vcpus / sockets))
    [ $((cores_per_socket * sockets)) -ne $total_vcpus ] && cores_per_socket=$total_vcpus && sockets=1

    # Per-vCPU pinning
    local cputune="" id=0
    for cpu in "${VCPU_PINS[@]}"; do
        cputune="$cputune,vcpupin$id.vcpu=$id,vcpupin$id.cpuset=$cpu"
        id=$((id + 1))
    done
    cputune=${cputune#,}
    local emu
    emu=$(expand_range "$HOST_CORES" | awk '{print $1}')
    [ -n "$emu" ] && cputune="$cputune,emulatorpin.cpuset=$emu"

    # NUMA cells + memnode pinning
    local cellcfg="" memnode="" mem_total_kb=0 i=0
    for i in "${!VM_NUMA_NODES[@]}"; do
        local node=${VM_NUMA_NODES[$i]}
        local cpus_start=$((cores_per_socket * i))
        local cpus_end=$((cores_per_socket * (i + 1) - 1))
        local mem_kb=$(( NUMA_HUGEPAGES_GB[$i] * 1024 * 1024 ))
        cellcfg="$cellcfg,numa.cell$i.cpus=$cpus_start-$cpus_end,numa.cell$i.memory=$mem_kb"
        memnode="$memnode,memnode$i.cellid=$i,memnode$i.mode=preferred,memnode$i.nodeset=$node"
        mem_total_kb=$((mem_total_kb + mem_kb))
    done

    local vm_nodeset
    vm_nodeset=$(IFS=,; echo "${VM_NUMA_NODES[*]}")

    local -a args=(
        --name "$VM_NAME"
        --memory "$((mem_total_kb / 1024))"
        --vcpus "$total_vcpus"
        --cputune "$cputune"
        --numatune "${vm_nodeset}${memnode}"
        --cpu "host-passthrough,+invtsc,topology.sockets=$sockets,topology.cores=$cores_per_socket,topology.threads=1,cache.level=3,cache.mode=emulate${cellcfg}"
        --memorybacking "hugepages=yes,hugepages.page.nodeset=$vm_nodeset,hugepages.page.size=1,hugepages.page.unit=G,nosharepages=yes,locked=yes,access.mode=private"
        --machine q35
        --os-variant linux2022
        --import
        --disk "path=$VM_IMAGE_PATH,format=qcow2,bus=virtio"
        --network "bridge=$MGMT_BRIDGE,model=virtio"
        --graphics none
        --console "pty,target_type=serial"
        --noautoconsole
        --memballoon none
    )

    # For multi-NUMA VMs only: emit a pcie-expander-bus per VM NUMA node and
    # attach hostdevs onto pcie-root-ports under it, so the guest sees each
    # passthrough NIC on the matching guest NUMA cell. Single-NUMA VMs have
    # only one possible cell, so libvirt's auto-allocation is correct.
    local pci
    if [ ${#VM_NUMA_NODES[@]} -gt 1 ]; then
        local rp_index=10
        local -A bus_for_cell=()
        local -A rp_for_cell=()
        for i in "${!VM_NUMA_NODES[@]}"; do
            local pxb_idx=$((100 + i))
            local target_bus=$((220 + i * 2))
            args+=( --controller "type=pci,index=$pxb_idx,model=pcie-expander-bus,target.busNr=$target_bus,target.node=$i" )
            bus_for_cell[$i]=$pxb_idx
        done

        for pci in "${PASSTHROUGH_INTERFACES[@]}"; do
            [ -z "$pci" ] && continue
            local hd pci_node vm_cell=0 idx=0
            hd=$(pci_to_hostdev "$pci")
            pci_node=$(pci_numa_node "$pci")
            for n in "${VM_NUMA_NODES[@]}"; do
                if [ "$n" = "$pci_node" ]; then vm_cell=$idx; break; fi
                idx=$((idx + 1))
            done
            args+=( --controller "type=pci,index=$rp_index,model=pcie-root-port,address.bus=${bus_for_cell[$vm_cell]}" )
            args+=( --hostdev "$hd,address.type=pci,address.bus=$rp_index,address.slot=0,address.function=0,rom.bar=off" )
            rp_index=$((rp_index + 1))
        done
    else
        for pci in "${PASSTHROUGH_INTERFACES[@]}"; do
            [ -z "$pci" ] && continue
            local hd
            hd=$(pci_to_hostdev "$pci")
            args+=( --hostdev "$hd" )
        done
    fi

    printf '%s\0' "${args[@]}"
}

create_vm() {
    if virsh list --all --name 2>/dev/null | grep -q "^${VM_NAME}$"; then
        if [ $INTERACTIVE -eq 1 ]; then
            $TUI_TOOL --title "VM exists" --yesno \
"VM '$VM_NAME' already exists. Destroy and recreate?" 8 60 || exit 1
        fi
        virsh destroy "$VM_NAME" 2>/dev/null || true
        virsh undefine "$VM_NAME" --nvram 2>/dev/null || virsh undefine "$VM_NAME" 2>/dev/null || true
    fi

    local xml=/tmp/${VM_NAME}.xml
    local -a args
    mapfile -d '' args < <(build_virtinstall_args)
    virt-install "${args[@]}" --print-xml > "$xml"
    virsh define "$xml" >/dev/null
    virsh autostart "$VM_NAME" >/dev/null
    echo "VM '$VM_NAME' defined. XML: $xml"
}

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
show_summary() {
    local passthrough_list pin_list mem_str numa_str
    passthrough_list=$(IFS=,; echo "${PASSTHROUGH_INTERFACES[*]:-(none)}")
    pin_list=$(IFS=,; echo "${VCPU_PINS[*]}")
    mem_str=$(IFS=,; echo "${NUMA_HUGEPAGES_GB[*]}")
    numa_str=$(IFS=,; echo "${VM_NUMA_NODES[*]}")

    cat <<EOF
==========================================================
  osvbng deployment summary
==========================================================
  VM name:           $VM_NAME
  Image:             $VM_IMAGE_PATH
  VM NUMA nodes:     $numa_str
  Memory (GB/node):  $mem_str   (1G hugepages)
  Host cores:        $HOST_CORES
  VM cores (vCPU 0..N): $pin_list
  Management bridge: $MGMT_BRIDGE
  Pass-through NICs: $passthrough_list
==========================================================
EOF

    if [ $REBOOT_NEEDED -eq 1 ]; then
        cat <<EOF
  >>> Reboot required to pick up new kernel cmdline.
  >>> After reboot, hugepages reserve automatically.
  >>> Then start the VM:
  >>>     virsh start $VM_NAME
==========================================================
EOF
    elif [ $DEFINE_VM -eq 1 ]; then
        cat <<EOF
  Start the VM:
    virsh start $VM_NAME
  Console:
    virsh console $VM_NAME
==========================================================
EOF
    fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
require_root

while getopts ":p:n:e:c:v:m:P:b:adsh" opt; do
    case $opt in
        p) VM_IMAGE_PATH=$OPTARG ;;
        n) VM_NAME=$OPTARG ;;
        e) NUMA_NODES=$OPTARG ;;
        c) HOST_CORES=$OPTARG ;;
        v) VM_CORES=$OPTARG ;;
        m) IFS=',' read -ra NUMA_HUGEPAGES_GB <<<"$OPTARG" ;;
        P) PASSTHROUGH_PCI=$OPTARG ;;
        b) MGMT_BRIDGE=$OPTARG ;;
        a) APPLY_HOST=1 ;;
        d) DEFINE_VM=1 ;;
        s) SWITCH_POWERSAVE=1 ;;
        h) usage; exit 0 ;;
        \?) echo "Unknown option: -$OPTARG" >&2; usage; exit 1 ;;
        :) echo "Option -$OPTARG requires an argument." >&2; exit 1 ;;
    esac
done
shift $((OPTIND - 1))

if [ "$OPTIND" -gt 1 ]; then
    INTERACTIVE=0
    # Materialize selections from flags
    if [ -n "$PASSTHROUGH_PCI" ]; then
        IFS=',' read -ra PASSTHROUGH_INTERFACES <<<"$PASSTHROUGH_PCI"
    fi
    if [ -z "$NUMA_NODES" ]; then
        echo "-e NUMA_NODES is required in non-interactive mode." >&2
        exit 1
    fi
    IFS=',' read -ra VM_NUMA_NODES <<<"$NUMA_NODES"
    if [ ${#NUMA_HUGEPAGES_GB[@]} -eq 0 ]; then
        echo "-m memory-per-NUMA is required in non-interactive mode." >&2
        exit 1
    fi
fi

detect_tui_tool
build_topology

if [ $INTERACTIVE -eq 1 ]; then
    show_banner
    check_requirements
    select_pci_devices
    prompt_vm_settings
fi

# Derive missing pieces
[ -z "$HOST_CORES" ] && auto_pick_host_cores
[ -z "$VM_CORES" ] && derive_vm_cores_from_host_reserve
build_vcpu_pin_list

[ -z "$VM_IMAGE_PATH" ] && VM_IMAGE_PATH="$INSTALL_DIR/${VM_NAME}.qcow2"

if [ $APPLY_HOST -eq 1 ]; then
    apply_host_config
fi

verify_iommu_groups_clean
verify_host_prepared || true

if [ $DEFINE_VM -eq 1 ]; then
    download_image
    create_vm
fi

show_summary
