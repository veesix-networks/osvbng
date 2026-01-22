# Configuration

osvbng uses a YAML configuration file located at `/etc/osvbng/osvbng.yaml`.

Generate a default config:

```bash
osvbngd config > /etc/osvbng/osvbng.yaml
```

## Sections

- [Logging](logging.md) - Log format and verbosity
- [Dataplane](dataplane.md) - DPDK and socket configuration
- [Interfaces](interfaces.md) - Network interfaces, VLANs, bonds
- [Subscriber Groups](subscriber-groups.md) - VLAN matching, address pools, per-group settings
- [DHCP](dhcp.md) - DHCP provider and pools
- [AAA](aaa.md) - Authentication and policies
- [Protocols](protocols.md) - BGP, OSPF, static routes
- [Monitoring](monitoring.md) - Metrics collection
- [Plugins](plugins.md) - Plugin configuration
