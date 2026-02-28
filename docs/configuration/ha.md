# High Availability

Active/standby HA configuration using Subscriber Redundancy Groups. See [architecture](../architecture/HA.md) for design details and failover behavior.

## Configuration

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `enabled` | bool | Enable HA | `true` |
| `node_id` | string | Unique node identifier (required) | `bng-1` |
| `listen` | [Listen](#listen) | gRPC listen address | |
| `peer` | [Peer](#peer) | Peer node connection | |
| `tls` | [TLS](#tls) | mTLS configuration | |
| `heartbeat` | [Heartbeat](#heartbeat) | Heartbeat timing | |
| `srgs` | map of [SRG](#srgs) | Subscriber Redundancy Groups (at least one required) | |

### Listen

| Field | Type | Default | Description | Example |
|-------|------|---------|-------------|---------|
| `address` | string | `:50051` | gRPC listen address in host:port format | `0.0.0.0:50051` |

### Peer

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `address` | string | Peer gRPC address in host:port format (required) | `172.30.0.3:50051` |

### TLS

All three fields required together, or omit entirely for plaintext.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `ca_cert` | string | CA certificate path | `/etc/osvbng/ha-ca.pem` |
| `cert` | string | Node certificate path | `/etc/osvbng/ha-cert.pem` |
| `key` | string | Node private key path | `/etc/osvbng/ha-key.pem` |

### Heartbeat

| Field | Type | Default | Description | Example |
|-------|------|---------|-------------|---------|
| `interval` | duration | `1s` | Heartbeat send interval | `1s` |
| `timeout` | duration | `5s` | Peer loss detection timeout (must be greater than interval) | `5s` |

### SRGs

Map keyed by SRG name.

| Field | Type | Default | Description | Example |
|-------|------|---------|-------------|---------|
| `virtual_mac` | string | | MAC address for dataplane programming | `02:00:5e:00:01:01` |
| `priority` | int | | Election priority, 1-255, higher wins (required) | `100` |
| `preempt` | bool | `false` | Re-elect when local priority exceeds current ACTIVE peer | `true` |
| `subscriber_groups` | []string | | Subscriber groups owned by this SRG (at least one required) | `[default]` |
| `interfaces` | []string | | Interfaces to track for priority adjustment | `[eth1]` |
| `track_priority_decrement` | int | `0` | Priority decrease per down tracked interface | `60` |
| `networks` | [][Network](#srg-networks) | | Prefixes to advertise/withdraw on failover | |

### SRG Networks

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `prefix` | string | IPv4 or IPv6 prefix in CIDR notation (required) | `10.255.0.0/16` |
| `vrf` | string | VRF for BGP advertisement (optional, omit for default VRF) | `CUSTOMER-A` |

## Example

Two-node deployment with interface tracking and route failover. bng-1 has higher priority (100) and is the preferred ACTIVE node. bng-2 has lower priority (50) with preempt enabled, so it takes over if bng-1's tracked interface goes down and its effective priority drops below 50.

**bng-1:**

```yaml
ha:
  enabled: true
  node_id: bng-1
  listen:
    address: 0.0.0.0:50051
  peer:
    address: 172.30.0.3:50051
  heartbeat:
    interval: 1s
    timeout: 5s
  srgs:
    default:
      virtual_mac: "02:00:5e:00:01:01"
      priority: 100
      preempt: false
      subscriber_groups:
        - default
        - pppoe
      interfaces:
        - TenGigabitEthernet1/0/0
      track_priority_decrement: 60
      networks:
        - prefix: 10.255.0.0/16
        - prefix: 2001:db8::/32
        - prefix: 10.100.0.0/16
          vrf: CUSTOMER-A
```

**bng-2:**

```yaml
ha:
  enabled: true
  node_id: bng-2
  listen:
    address: 0.0.0.0:50051
  peer:
    address: 172.30.0.2:50051
  heartbeat:
    interval: 1s
    timeout: 5s
  srgs:
    default:
      virtual_mac: "02:00:5e:00:01:01"
      priority: 50
      preempt: true
      subscriber_groups:
        - default
        - pppoe
      interfaces:
        - TenGigabitEthernet1/0/0
      track_priority_decrement: 60
      networks:
        - prefix: 10.255.0.0/16
        - prefix: 2001:db8::/32
        - prefix: 10.100.0.0/16
          vrf: CUSTOMER-A
```

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/show/ha/status` | GET | HA status: enabled, node ID, peer state, SRG summary |
| `/api/show/ha/srg` | GET | SRG details: state, priority, base priority, preempt, virtual MAC, tracked/down interfaces |
| `/api/show/ha/peer` | GET | Peer info: connected, node ID, last heartbeat, RTT, clock skew |
| `/api/show/ha/srg/counters` | GET | Dataplane counters: GARP sent, NA sent, MAC adds/removes |
| `/api/exec/ha/switchover` | POST | Trigger graceful switchover (optional JSON body with `srg_names` array to limit scope) |
