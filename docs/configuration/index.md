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
- [Subscriber Groups](subscriber-groups.md) - VLAN matching and per-group settings
- [IPv4 Profiles](ipv4-profiles.md) - Address pools, gateway, DNS, and DHCP options
- [IPv6 Profiles](ipv6-profiles.md) - IANA pools, prefix delegation, and DHCPv6 options
- [DHCP](dhcp.md) - DHCPv4 provider configuration
- [DHCPv6](dhcpv6.md) - DHCPv6 provider configuration
- [AAA](aaa.md) - Authentication and policies
- [QoS Policies](qos.md) - Per-subscriber rate limiting with VPP policers
- [Service Groups](service-groups.md) - Named per-subscriber attribute bundles (VRF, ACL, QoS)
- [Subscriber Provisioning](provisioning.md) - AAA attribute resolution, override priority, and scenarios
- [VRFs](vrfs.md) - Virtual Routing and Forwarding instances
- [Protocols](protocols.md) - BGP, OSPF, OSPFv3, IS-IS, static routes
- [MPLS](mpls.md) - LDP, Segment Routing, SRv6 (in development)
- [System](system.md) - System-level settings and control plane protection
- [Monitoring](monitoring.md) - Metrics collection
- [Plugins](plugins.md) - Plugin configuration
