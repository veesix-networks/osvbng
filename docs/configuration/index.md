# Configuration

osvbng uses a YAML configuration file located at `/etc/osvbng/osvbng.yaml`.

Generate a default config:

```bash
osvbngd config > /etc/osvbng/osvbng.yaml
```

## Sections

- [Logging](logging.md) - Log format and verbosity
- [Dataplane](dataplane.md) - DPDK and stats segment configuration
- [Interfaces](interfaces.md) - Network interfaces and sub-interfaces
- [Subscriber Groups](subscriber-groups.md) - VLAN matching, address pools, per-group settings
- [DHCP](dhcp.md) - DHCPv4 provider and pools
- [DHCPv6](dhcpv6.md) - DHCPv6 provider, IANA pools, and prefix delegation
- [AAA](aaa.md) - Authentication and policies
- [Service Groups](service-groups.md) - Named per-subscriber attribute bundles (VRF, ACL, QoS)
- [VRFs](vrfs.md) - Virtual Routing and Forwarding instances
- [Protocols](protocols.md) - BGP, OSPF, OSPFv3, IS-IS, static routes
- [MPLS](mpls.md) - LDP, Segment Routing, SRv6 (in development)
- [System](system.md) - System-level settings and control plane protection
- [Monitoring](monitoring.md) - Metrics collection
- [Plugins](plugins.md) - Plugin configuration
