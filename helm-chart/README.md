# OSVBNG Helm Chart Deployment Guide

## Overview

Deploy a complete virtual BNG (Broadband Network Gateway) solution on Kubernetes with OSVBNG and BNG Blaster for IPoE/PPPoE subscriber testing.

## Prerequisites

### Required Components
- Kubernetes cluster (1.31+)
- Multus CNI installed
- SR-IOV or host-device capable network interfaces
- Hugepages support enabled on worker nodes

### Host Network Interfaces (Adjust to fit your network layout)
Configure the following physical interfaces on your K8s worker nodes:

**OSVBNG Pod:**
- `ens21` - Access interface (subscriber-facing)
- `ens22` - Core/uplink interface

**BNG Blaster Pod:**
- `ens19` - Access interface (subscriber simulation)
- `ens20` - Core interface

### Node Configuration

Enable hugepages on worker nodes:
```bash
# Configure 512 x 2MB hugepages (1G total)
echo 512 > /sys/kernel/mm/hugepages/hugepages-2048kB/nr_hugepages

# Make persistent (add to /etc/sysctl.conf)
vm.nr_hugepages = 512
```

Enable static CPU manager policy to enforce CPU pinning (and guranteed QoS at the POD level) at the kubelet level, example way of doing this in mikrok8s:

```
sudo cat /var/snap/microk8s/current/args/kubelet
--resolv-conf=/run/systemd/resolve/resolv.conf
--kubeconfig=${SNAP_DATA}/credentials/kubelet.config
--cert-dir=${SNAP_DATA}/certs
--client-ca-file=${SNAP_DATA}/certs/ca.crt
--anonymous-auth=false
--root-dir=${SNAP_COMMON}/var/lib/kubelet
--fail-swap-on=false
--eviction-hard="memory.available<100Mi,nodefs.available<1Gi,imagefs.available<1Gi"
--container-runtime-endpoint=${SNAP_COMMON}/run/containerd.sock
--containerd=${SNAP_COMMON}/run/containerd.sock
--node-labels="microk8s.io/cluster=true,node.kubernetes.io/microk8s-controlplane=microk8s-controlplane"
--authentication-token-webhook=true
--read-only-port=0
--tls-cipher-suites=TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,TLS_RSA_WITH_AES_256_GCM_SHA384,TLS_RSA_WITH_AES_128_GCM_SHA256
--serialize-image-pulls=false
--cluster-domain=cluster.local
--cluster-dns=10.152.183.10
--cpu-manager-policy=static
--reserved-cpus=0-5
```

The last two lines are of particular interest:

**--cpu-manager-policy=static**
**--reserved-cpus=0-5**

## Helm Values Configuration

The chart supports customization through `values.yaml`:

```yaml
osvbng:
  image: "ghcr.io/infinitydon/osvbng:v0.0.3"
  pullPolicy: IfNotPresent #Always
  accessInterface: "net1"
  coreInterface: "net2"
  privileged: true
  serviceType: ClusterIP
  ports:
    grpc: 50050
    prometheus: 9090
    http: 8080
  mountSysKernelDebug: true
  mountHugepages: true
  shmSize: "2Gi"
  resources:
    cpu: 4
    memory: "6Gi"

bngblaster:
  enabled: true
  image: "ghcr.io/infinitydon/bng-blaster:v0.9.30"
  pullPolicy: IfNotPresent
  accessInterface: "net1"
  coreInterface: "net2"

multus:
  enabled: true
  osvbng:
    devices:
      accessHostDevice: "ens21"
      coreHostDevice: "ens22"
  bngblaster:
    devices:
      accessHostDevice: "ens19"
      coreHostDevice: "ens20"
```

## Deployment

### Install/Upgrade the Chart

```bash
helm upgrade --install virtual-bng ./osvbng-chart/ \
  --namespace telco \
  --create-namespace
```

### Verify Deployment

Check pod status:
```bash
kubectl -n telco get pods

# Expected output:
# NAME                         READY   STATUS    RESTARTS   AGE
# bngblaster-984f4b759-gsqjg   1/1     Running   0          7m17s
# osvbng-0                     1/1     Running   0          7m17s
```

Check OSVBNG logs:
```bash
kubectl -n telco logs osvbng-0 | head -20

# Expected output shows CPU allocation:
# CPUs allowed by cgroup: 6-9
# Total cores allocated to pod: 4
# Core allocation: Total=4 DP_MAIN=6 DP_WORKERS=7-9 CP=6
```

## Testing & Validation

### 1. Check OSVBNG VPP Status

```bash
kubectl -n telco exec -ti osvbng-0 -- vppctl -s /run/osvbng/cli.sock

# Inside VPP CLI:
osvbng# show int
osvbng# show int addr
```

**Expected interfaces:**
- `host-eth0` - Management interface
- `host-net1` - Access interface (connected to BNG Blaster)
- `host-net2` - Core interface
- `loop100` - Subscriber gateway (10.255.0.1/32)
- `memif1/0` - Internal dataplane interface

### 2. Run BNG Blaster Test

```bash
kubectl -n telco exec -ti deploy/bngblaster -- bash

# Inside BNG Blaster container:
bngblaster -C /config-templates/multiple-multi-qinq.json -c 10 -I -l dhcp -l ip
```

**Test Parameters:**
- `-c 10` - Create 10 subscriber sessions
- `-I` - Interactive mode
- `-l dhcp` - Log DHCP protocol
- `-l ip` - Log IP protocol

**Expected Results:**
```
Sessions PPPoE: 0 IPoE: 10
Sessions established: 10/10
Setup Time: 949 ms
Setup Rate: 10.54 CPS
```

### 3. Verify Active Sessions

```bash
kubectl -n telco exec -ti osvbng-0 -- osvbngcli

# Inside osvbngcli:
bng> show subscriber sessions
```

**Expected output:** Active IPoE sessions with:
- MAC addresses (02:00:00:00:00:XX)
- IPv4 addresses from pool (10.255.0.0/16)
- DHCP lease time (3600s)
- State: `active`

### 4. Check Network Interfaces

```bash
kubectl -n telco exec -ti osvbng-0 -- ip addr

# Expected interfaces:
# - eth0: K8s cluster network
# - net1: Access interface (from Multus)
# - net2: Core interface (from Multus)
# - loop100: Subscriber gateway (10.255.0.1/32)
```

## Architecture

```
┌─────────────────┐         ┌─────────────────┐
│  BNG Blaster    │         │     OSVBNG      │
│                 │         │                 │
│  ens19 (net1) ◄─┼─────────┼─► ens21 (net1)  │
│  Subscriber     │  Access │   Access        │
│  Simulation     │  VLAN   │   Interface     │
│                 │   100   │                 │
│  ens20 (net2) ◄─┼─────────┼─► ens22 (net2)  │
│  Core           │  Core   │   Uplink        │
└─────────────────┘         └─────────────────┘
```

## Subscriber Pool Configuration

**Default Address Pool:** `10.255.0.0/16`
- Gateway: `10.255.0.1`
- DNS: `8.8.8.8`, `8.8.4.4`
- DHCP Lease: 3600 seconds

**VLAN Configuration:**
- S-VLAN: 100
- C-VLAN: any (or specific per subscriber)

## Monitoring

### Prometheus Metrics
```bash
kubectl -n telco port-forward osvbng-0 9090:9090
curl http://localhost:9090/metrics
```

### OSVBNG CLI
```bash
kubectl -n telco exec -ti osvbng-0 -- osvbngcli

# Available commands:
bng> show subscriber sessions
bng> show interfaces
bng> help
```

### VPP CLI
```bash
kubectl -n telco exec -ti osvbng-0 -- vppctl -s /run/osvbng/cli.sock

# Useful commands:
osvbng# show int
osvbng# show int addr
osvbng# show hardware-interfaces
osvbng# show errors
```

## Troubleshooting

### No Subscriber Sessions

1. **Check VPP interfaces:**
   ```bash
   kubectl -n telco exec osvbng-0 -- vppctl -s /run/osvbng/cli.sock show int
   # Ensure host-net1 is UP
   ```

2. **Verify Multus attachments:**
   ```bash
   kubectl -n telco get network-attachment-definitions
   kubectl -n telco describe pod osvbng-0 | grep -A 5 "Annotations"
   ```

3. **Check DHCP logs:**
   ```bash
   kubectl -n telco logs osvbng-0 | grep dhcp
   ```

### VPP Not Starting

1. **Check CPU allocation:**
   ```bash
   kubectl -n telco logs osvbng-0 | grep "Core allocation"
   ```

2. **Verify hugepages:**
   ```bash
   kubectl -n telco describe pod osvbng-0 | grep -i hugepage
   ```

3. **Check dataplane logs:**
   ```bash
   kubectl -n telco exec osvbng-0 -- tail -50 /var/log/osvbng/dataplane.log
   ```

### Interface Not Found

```bash
# Verify host interfaces exist
ssh <worker-node>
ip link show ens21
ip link show ens22

# Check Multus NAD configuration
kubectl -n telco get network-attachment-definitions -o yaml
```

## Cleanup

```bash
# Uninstall the chart
helm -n telco uninstall virtual-bng

# Delete namespace
kubectl delete namespace telco
```

## Advanced Configuration

### Custom Subscriber Pool

Edit `osvbng-config-template` ConfigMap:
```yaml
address-pools:
  - name: subscriber-pool
    network: 192.168.0.0/16  # Custom pool
    gateway: 192.168.0.1
```

### BGP Configuration

Enable BGP in `osvbng-config-template`:
```yaml
protocols:
  bgp:
    asn: 65000
    router-id: 10.255.0.1
    neighbors:
      - ip: 10.0.0.1
        asn: 65001
```

### Multiple Subscriber Groups

Add additional groups in `osvbng.yaml.template`:
```yaml
subscriber-groups:
  groups:
    premium:
      vlans:
        - svlan: "200"
          cvlan: any
      address-pools:
        - name: premium-pool
          network: 10.10.0.0/16
```

## References

- **BNG Blaster:** https://github.com/rtbrick/bngblaster
- **Multus CNI:** https://github.com/k8snetworkplumbingwg/multus-cni