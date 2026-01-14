#
<p align="center">
  <img src="docs/img/logo.png" alt="Logo" style="max-width: 25%; height: auto;">
</p>

<a href="https://github.com/veesix-networks/osvbng" target="_blank">osvbng</a> (Open Source Virtual Broadband Network Gateway) is a high-performance, scalable, open source BNG for ISPs. Built to scale up to multi-hundred gigabit throughput on standard x86 COTS hardware.

## Key Features

- 400+Gbps throughput with Intel DPDK (Up to 100+Gbps without DPDK)
- 20,000+ Subscriber Sessions
- Plugin-based architecture
- IPoE/DHCPv4
- Modern monitoring stack
- Core implementation is fully open source
- Docker and KVM support

## Get Started

### Quick Start with Docker

**Prerequisites:**

- Docker installed
- Minimum of 2 physical network interfaces (access and core) if deploying in a non-test scenario

**Step 1: Start the container**

```bash
docker run -d --name osvbng \
  --privileged \
  --network none \
  -e OSVBNG_WAIT_FOR_INTERFACES=true \
  -e OSVBNG_ACCESS_INTERFACE=eth0 \
  -e OSVBNG_CORE_INTERFACE=eth1 \
  veesixnetworks/osvbng:latest
```

**Step 2: Attach network interfaces**

For production with physical NICs (replace `enp0s1` and `enp1s1` with your interface names):
```bash
curl -sLO https://raw.githubusercontent.com/veesix-networks/osvbng/dev/docker/setup-interfaces.sh
chmod +x setup-interfaces.sh
./setup-interfaces.sh osvbng eth0:enp0s1 eth1:enp1s1
```

For testing without physical hardware:
```bash
./setup-interfaces.sh osvbng eth0 eth1
```

!!! tip

    Container network interfaces must be recreated after each restart. The `setup-interfaces.sh` script creates veth pairs (virtual ethernet pairs) that connect your host's physical interfaces to the container. A veth pair acts like a virtual cable with two ends - one end stays on the host and is bridged to your physical NIC, while the other end is moved into the container's network namespace.

    Network namespaces are tied to process IDs, which are allocated by the kernel on container start and cannot be predicted. When a container restarts, it gets a new PID and new namespace, breaking the connection to the old veth pair. This is why the veth pairs must be recreated after every restart.

    For "production" deployments, use the systemd service (or equivalent) to automatically handle interface setup on container restart.

**Step 3: Verify it's running**

```bash
docker logs -f osvbng
```

**Step 4: Access the CLI**

```bash
docker exec -it osvbng osvbngcli
```

#### "Production" Deployment

For "production" deployments, you need to ensure the container and its network interfaces are automatically set up on system boot. Below is an example using systemd (adjust for your init system if using something else):

```bash
# Download the service file and setup script
curl -sLO https://raw.githubusercontent.com/veesix-networks/osvbng/dev/docker/osvbng.service
curl -sLO https://raw.githubusercontent.com/veesix-networks/osvbng/dev/docker/setup-interfaces.sh

# Create working directory
sudo mkdir -p /opt/osvbng
sudo mv setup-interfaces.sh /opt/osvbng/
sudo chmod +x /opt/osvbng/setup-interfaces.sh

# Edit the service file and update the interface names on line 25
# Change eth0:ens19 eth1:ens20 to match your host interface names
sudo nano osvbng.service

# Install and enable the service
sudo mv osvbng.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable osvbng.service
sudo systemctl start osvbng.service
```

Check service status:
```bash
sudo systemctl status osvbng.service
```

#### Customizing Configuration

Generate and customize the config file:

```bash
docker run --rm veesixnetworks/osvbng:latest config > osvbng.yaml
sudo mv osvbng.yaml /etc/osvbng/
```

Mount it into the container:

```bash
docker run -d --name osvbng \
  --privileged \
  --network none \
  -v /opt/osvbng/osvbng.yaml:/etc/osvbng/osvbng.yaml:ro \
  -e OSVBNG_WAIT_FOR_INTERFACES=true \
  -e OSVBNG_ACCESS_INTERFACE=eth0 \
  -e OSVBNG_CORE_INTERFACE=eth1 \
  veesixnetworks/osvbng:latest
```

Or update the systemd service file to include the volume mount.

### QEMU / KVM with DPDK

For maximum performance with DPDK and PCI passthrough. Requires a dedicated server with:

- KVM/libvirt installed
- IOMMU enabled (Intel VT-d or AMD-Vi)
- At least 2 physical NICs for PCI passthrough (access and core)
- 4GB+ RAM and 4+ CPU cores recommended

**Quick Install:**

```bash
curl -sL https://v6n.io/osvbng | sudo bash
```

**Install Dependencies (Debian/Ubuntu):**

```bash
sudo apt install -y libvirt-daemon-system qemu-kvm virtinst curl whiptail gzip
```

**Manual Install:**

```bash
# Download the deployment script
curl -sLO https://raw.githubusercontent.com/veesix-networks/osvbng/dev/scripts/qemu/deploy-vm.sh
chmod +x deploy-vm.sh

# Run the interactive installer
sudo ./deploy-vm.sh
```

The installer will:

1. Check prerequisites (IOMMU, vfio-pci, etc.)
2. Let you select NICs for PCI passthrough (access and core)
3. Download and deploy the osvbng VM image
4. Configure the VM with your selected interfaces

**Manual Image Download (Optional):**

If you prefer to download the QEMU image separately, you can download the qcow2 images manually from the Releases page or run the following:

```bash
# Download latest release
curl -fLO https://github.com/veesix-networks/osvbng/releases/latest/download/osvbng-debian12.qcow2.gz

# Or download a specific version
curl -fLO https://github.com/veesix-networks/osvbng/releases/download/v1.0.0/osvbng-debian12.qcow2.gz

# Extract the image
gunzip osvbng-debian12.qcow2.gz

# Move to libvirt images directory
sudo mv osvbng-debian12.qcow2 /var/lib/libvirt/images/osvbng.qcow2
```

**Start and Connect:**

```bash
# Start the VM
sudo virsh start osvbng

# Connect to console
sudo virsh console osvbng

# Default login: root / osvbng
```

**Access the CLI:**

```bash
# Inside the VM
osvbngcli

osvbng> show subscriber sessions
```

The VM auto-generates a default configuration on first boot. To customize, edit `/etc/osvbng/osvbng.yaml` and restart the osvbng service.

#### Tested Host Operating Systems

- Ubuntu 22.04, 24.04
- Debian 12 (Bookworm)

## Expectations

What can you expect from the open source version of this project? Below are some key points we want to always achieve in every major release:

- Minimum of 100Gbps out-of-the-box support
- IPoE access technology with DHCPv4 support
- Authenticate customers via DHCPv4 Option 82 (Sub-options 1 and 2, Circuit ID and/or Remote ID)
- BGP, IS-IS and OSPF support
- Only Default VRF implementation
- No QoS/HQoS support from day 1 of the v1.0.0 release
- Modern monitoring solution with Prometheus