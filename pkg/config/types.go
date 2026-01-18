package config

import (
	"time"
)

type Config struct {
	Logging         LoggingConfig               `json:"logging,omitempty" yaml:"logging,omitempty"`
	Dataplane       DataplaneConfig             `json:"dataplane,omitempty" yaml:"dataplane,omitempty"`
	SubscriberGroup SubscriberGroupConfig       `json:"subscriber_group,omitempty" yaml:"subscriber_group,omitempty"`
	DHCP            DHCPConfig                  `json:"dhcp,omitempty" yaml:"dhcp,omitempty"`
	Monitoring      MonitoringConfig            `json:"monitoring,omitempty" yaml:"monitoring,omitempty"`
	API             APIConfig                   `json:"api,omitempty" yaml:"api,omitempty"`
	Interfaces      map[string]*InterfaceConfig `json:"interfaces,omitempty" yaml:"interfaces,omitempty"`
	Protocols       ProtocolConfig              `json:"protocols,omitempty" yaml:"protocols,omitempty"`
	AAA             AAAConfig                   `json:"aaa,omitempty" yaml:"aaa,omitempty"`
	VRFS            []VRFSConfig                `json:"vrfs,omitempty" yaml:"vrfs,omitempty"`
	Plugins         map[string]interface{}      `json:"plugins,omitempty" yaml:"plugins,omitempty"`
}

type VRFSConfig struct {
	Name               string                 `json:"name" yaml:"name"`
	Description        string                 `json:"description,omitempty" yaml:"description,omitempty"`
	RouteDistinguisher string                 `json:"rd,omitempty" yaml:"rd,omitempty"`
	ImportRouteTargets []string               `json:"import-route-targets,omitempty" yaml:"import-route-targets,omitempty"`
	ExportRouteTargets []string               `json:"export-route-targets,omitempty" yaml:"export-route-targets,omitempty"`
	AddressFamilies    VRFAddressFamilyConfig `json:"address-families,omitempty" yaml:"address-families,omitempty"`
}

type VRFAddressFamilyConfig struct {
	IPv4Unicast *IPv4UnicastConfig `json:"ipv4-unicast,omitempty" yaml:"ipv4-unicast,omitempty"`
	IPv6Unicast *IPv6UnicastConfig `json:"ipv6-unicast,omitempty" yaml:"ipv6-unicast,omitempty"`
}

type IPv4UnicastConfig struct {
	ImportRoutePolicy string `json:"import-route-policy,omitempty" yaml:"import-route-policy,omitempty"`
	ExportRoutePolicy string `json:"export-route-policy,omitempty" yaml:"export-route-policy,omitempty"`
}

type IPv6UnicastConfig struct {
	ImportRoutePolicy string `json:"import-route-policy,omitempty" yaml:"import-route-policy,omitempty"`
	ExportRoutePolicy string `json:"export-route-policy,omitempty" yaml:"export-route-policy,omitempty"`
}

type InterfaceConfig struct {
	Name        string         `json:"name" yaml:"name"`
	Description string         `json:"description,omitempty" yaml:"description,omitempty"`
	Enabled     bool           `json:"enabled" yaml:"enabled"`
	MTU         int            `json:"mtu,omitempty" yaml:"mtu,omitempty"`
	Address     *AddressConfig `json:"address,omitempty" yaml:"address,omitempty"`

	Type   string      `json:"type,omitempty" yaml:"type,omitempty"`
	Parent string      `json:"parent,omitempty" yaml:"parent,omitempty"`
	VLANID int         `json:"vlan-id,omitempty" yaml:"vlan-id,omitempty"`
	Bond   *BondConfig `json:"bond,omitempty" yaml:"bond,omitempty"`
	LCP    bool        `json:"lcp,omitempty" yaml:"lcp,omitempty"`
}

type AddressConfig struct {
	IPv4 []string `json:"ipv4,omitempty" yaml:"ipv4,omitempty"`
	IPv6 []string `json:"ipv6,omitempty" yaml:"ipv6,omitempty"`
}

type BondConfig struct {
	Mode    string   `json:"mode" yaml:"mode"`
	Members []string `json:"members" yaml:"members"`
	MIIMon  int      `json:"miimon,omitempty" yaml:"miimon,omitempty"`
}

type ProtocolConfig struct {
	BGP    *BGPConfig    `json:"bgp,omitempty" yaml:"bgp,omitempty"`
	OSPF   *OSPFConfig   `json:"ospf,omitempty" yaml:"ospf,omitempty"`
	Static *StaticConfig `json:"static,omitempty" yaml:"static,omitempty"`
}

type StaticConfig struct {
	IPv4 []StaticRoute `json:"ipv4,omitempty" yaml:"ipv4,omitempty"`
	IPv6 []StaticRoute `json:"ipv6,omitempty" yaml:"ipv6,omitempty"`
}

type BGPConfig struct {
	ASN         uint32                   `json:"asn" yaml:"asn"`
	RouterID    string                   `json:"router-id,omitempty" yaml:"router-id,omitempty"`
	Neighbors   map[string]*BGPNeighbor  `json:"neighbors,omitempty" yaml:"neighbors,omitempty"`
	IPv4Unicast *BGPAddressFamily        `json:"ipv4-unicast,omitempty" yaml:"ipv4-unicast,omitempty"`
	IPv6Unicast *BGPAddressFamily        `json:"ipv6-unicast,omitempty" yaml:"ipv6-unicast,omitempty"`
	VRF         map[string]*BGPVRFConfig `json:"vrf,omitempty" yaml:"vrf,omitempty"`
}

type BGPAddressFamily struct {
	Neighbors map[string]*BGPNeighbor `json:"neighbors,omitempty" yaml:"neighbors,omitempty"`
	Networks  []string                `json:"networks,omitempty" yaml:"networks,omitempty"`
}

type BGPVRFConfig struct {
	RouterID    string            `json:"router-id,omitempty" yaml:"router-id,omitempty"`
	RD          string            `json:"rd,omitempty" yaml:"rd,omitempty"`
	IPv4Unicast *BGPAddressFamily `json:"ipv4-unicast,omitempty" yaml:"ipv4-unicast,omitempty"`
	IPv6Unicast *BGPAddressFamily `json:"ipv6-unicast,omitempty" yaml:"ipv6-unicast,omitempty"`
}

type BGPNeighbor struct {
	Peer        string `json:"peer,omitempty" yaml:"peer,omitempty"`
	RemoteAS    uint32 `json:"remote_as,omitempty" yaml:"remote_as,omitempty"`
	BFD         bool   `json:"bfd,omitempty" yaml:"bfd,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

type OSPFConfig struct {
	RouterID string        `json:"router-id" yaml:"router-id"`
	Networks []OSPFNetwork `json:"networks,omitempty" yaml:"networks,omitempty"`
}

type OSPFNetwork struct {
	Prefix string `json:"prefix" yaml:"prefix"`
	Area   string `json:"area" yaml:"area"`
}

type StaticRoute struct {
	Destination string `json:"destination" yaml:"destination"`
	NextHop     string `json:"next-hop,omitempty" yaml:"next-hop,omitempty"`
	Device      string `json:"device,omitempty" yaml:"device,omitempty"`
}

type DiffResult struct {
	Added    []ConfigLine
	Deleted  []ConfigLine
	Modified []ConfigLine
}

type ConfigLine struct {
	Path  string
	Value string
}

type AAAConfig struct {
	Provider      string      `json:"provider,omitempty" yaml:"provider,omitempty"`
	NASIdentifier string      `json:"nas_identifier,omitempty" yaml:"nas_identifier,omitempty"`
	NASIP         string      `json:"nas_ip,omitempty" yaml:"nas_ip,omitempty"`
	Policy        []AAAPolicy `json:"policy,omitempty" yaml:"policy,omitempty"`
}

type AAAPolicy struct {
	Name                  string `json:"name" yaml:"name"`
	Format                string `json:"format,omitempty" yaml:"format,omitempty"`
	Type                  string `json:"type,omitempty" yaml:"type,omitempty"`
	MaxConcurrentSessions int    `json:"max_concurrent_sessions,omitempty" yaml:"max_concurrent_sessions,omitempty"`
}

func (a *AAAConfig) GetPolicy(name string) *AAAPolicy {
	for i := range a.Policy {
		if a.Policy[i].Name == name {
			return &a.Policy[i]
		}
	}
	return nil
}

type LoggingConfig struct {
	Format     string            `json:"format,omitempty" yaml:"format,omitempty"`
	Level      string            `json:"level,omitempty" yaml:"level,omitempty"`
	Components map[string]string `json:"components,omitempty" yaml:"components,omitempty"`
}

type DataplaneConfig struct {
	AccessInterface   string     `json:"access_interface,omitempty" yaml:"access_interface,omitempty"`
	DPAPISocket       string     `json:"dp_api_socket,omitempty" yaml:"dp_api_socket,omitempty"`
	PuntSocketPath    string     `json:"punt_socket_path,omitempty" yaml:"punt_socket_path,omitempty"`
	ARPPuntSocketPath string     `json:"arp_punt_socket_path,omitempty" yaml:"arp_punt_socket_path,omitempty"`
	MemifSocketPath   string     `json:"memif_socket_path,omitempty" yaml:"memif_socket_path,omitempty"`
	DPDK              *DPDKConfig `json:"dpdk,omitempty" yaml:"dpdk,omitempty"`
}

type DPDKConfig struct {
	UIODriver            string              `json:"uio_driver,omitempty" yaml:"uio_driver,omitempty"`
	Devices              []DPDKDevice        `json:"devices,omitempty" yaml:"devices,omitempty"`
	DevDefaults          *DPDKDeviceOptions  `json:"dev_defaults,omitempty" yaml:"dev_defaults,omitempty"`
	SocketMem            string              `json:"socket_mem,omitempty" yaml:"socket_mem,omitempty"`
	NoMultiSeg           bool                `json:"no_multi_seg,omitempty" yaml:"no_multi_seg,omitempty"`
	NoTxChecksumOffload  bool                `json:"no_tx_checksum_offload,omitempty" yaml:"no_tx_checksum_offload,omitempty"`
	EnableTcpUdpChecksum bool                `json:"enable_tcp_udp_checksum,omitempty" yaml:"enable_tcp_udp_checksum,omitempty"`
	MaxSimdBitwidth      int                 `json:"max_simd_bitwidth,omitempty" yaml:"max_simd_bitwidth,omitempty"`
}

type DPDKDevice struct {
	PCI     string              `json:"pci" yaml:"pci"`
	Name    string              `json:"name,omitempty" yaml:"name,omitempty"`
	Options *DPDKDeviceOptions  `json:"options,omitempty" yaml:"options,omitempty"`
}

type DPDKDeviceOptions struct {
	NumRxQueues   int    `json:"num_rx_queues,omitempty" yaml:"num_rx_queues,omitempty"`
	NumTxQueues   int    `json:"num_tx_queues,omitempty" yaml:"num_tx_queues,omitempty"`
	NumRxDesc     int    `json:"num_rx_desc,omitempty" yaml:"num_rx_desc,omitempty"`
	NumTxDesc     int    `json:"num_tx_desc,omitempty" yaml:"num_tx_desc,omitempty"`
	TSO           bool   `json:"tso,omitempty" yaml:"tso,omitempty"`
	Devargs       string `json:"devargs,omitempty" yaml:"devargs,omitempty"`
	RssQueues     string `json:"rss_queues,omitempty" yaml:"rss_queues,omitempty"`
	NoRxInterrupt bool   `json:"no_rx_interrupt,omitempty" yaml:"no_rx_interrupt,omitempty"`
}

type SubscriberGroupConfig struct {
	DefaultPolicy string      `json:"default_policy,omitempty" yaml:"default_policy,omitempty"`
	VLANs         []VLANRange `json:"vlans,omitempty" yaml:"vlans,omitempty"`
}

func (sg *SubscriberGroupConfig) FindGatewayForSVLAN(svlan uint16) string {
	for _, vlanCfg := range sg.VLANs {
		if vlanCfg.MatchesSVLAN(svlan) {
			return vlanCfg.Interface
		}
	}
	return ""
}

func (sg *SubscriberGroupConfig) FindVLANConfig(svlan uint16) *VLANRange {
	for i := range sg.VLANs {
		if sg.VLANs[i].MatchesSVLAN(svlan) {
			return &sg.VLANs[i]
		}
	}
	return nil
}

func (sg *SubscriberGroupConfig) GetPolicyName(svlan uint16) string {
	vlanCfg := sg.FindVLANConfig(svlan)
	if vlanCfg != nil && vlanCfg.AAA != nil && vlanCfg.AAA.Policy != "" {
		return vlanCfg.AAA.Policy
	}
	return sg.DefaultPolicy
}

type VLANRange struct {
	SVLAN     string    `json:"svlan,omitempty" yaml:"svlan,omitempty"`
	CVLAN     string    `json:"cvlan,omitempty" yaml:"cvlan,omitempty"`
	Interface string    `json:"interface,omitempty" yaml:"interface,omitempty"`
	IPv4      []string  `json:"ipv4,omitempty" yaml:"ipv4,omitempty"`
	IPv6      []string  `json:"ipv6,omitempty" yaml:"ipv6,omitempty"`
	DHCP      string    `json:"dhcp,omitempty" yaml:"dhcp,omitempty"`
	AAA       *VLANAAAs `json:"aaa,omitempty" yaml:"aaa,omitempty"`
	Template  string    `json:"template,omitempty" yaml:"template,omitempty"`
}

func (v *VLANRange) GetSVLANs() ([]uint16, error) {
	return parseVLANRange(v.SVLAN)
}

func (v *VLANRange) GetCVLAN() (isAny bool, cvlan uint16, err error) {
	return parseCVLAN(v.CVLAN)
}

func (v *VLANRange) MatchesSVLAN(svlan uint16) bool {
	svlans, err := v.GetSVLANs()
	if err != nil {
		return false
	}
	for _, s := range svlans {
		if s == svlan {
			return true
		}
	}
	return false
}

type VLANAAAs struct {
	Enabled bool   `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Policy  string `json:"policy,omitempty" yaml:"policy,omitempty"`
}

type DHCPConfig struct {
	Provider      string       `json:"provider,omitempty" yaml:"provider,omitempty"`
	DefaultServer string       `json:"default_server,omitempty" yaml:"default_server,omitempty"`
	Servers       []DHCPServer `json:"servers,omitempty" yaml:"servers,omitempty"`
	Pools         []DHCPPool   `json:"pools,omitempty" yaml:"pools,omitempty"`
}

func (d *DHCPConfig) GetServer(name string) *DHCPServer {
	for i := range d.Servers {
		if d.Servers[i].Name == name {
			return &d.Servers[i]
		}
	}
	return nil
}

type DHCPServer struct {
	Name    string `json:"name,omitempty" yaml:"name,omitempty"`
	Address string `json:"address,omitempty" yaml:"address,omitempty"`
	GIAddr  string `json:"giaddr,omitempty" yaml:"giaddr,omitempty"`
}

type DHCPPool struct {
	Name       string   `json:"name,omitempty" yaml:"name,omitempty"`
	Network    string   `json:"network,omitempty" yaml:"network,omitempty"`
	RangeStart string   `json:"range_start,omitempty" yaml:"range_start,omitempty"`
	RangeEnd   string   `json:"range_end,omitempty" yaml:"range_end,omitempty"`
	Gateway    string   `json:"gateway,omitempty" yaml:"gateway,omitempty"`
	DNSServers []string `json:"dns_servers,omitempty" yaml:"dns_servers,omitempty"`
	LeaseTime  uint32   `json:"lease_time,omitempty" yaml:"lease_time,omitempty"`
}

type MonitoringConfig struct {
	DisabledCollectors []string      `json:"disabled_collectors,omitempty" yaml:"disabled_collectors,omitempty"`
	CollectInterval    time.Duration `json:"collect_interval,omitempty" yaml:"collect_interval,omitempty"`
}

type APIConfig struct {
	Address string `json:"address,omitempty" yaml:"address,omitempty"`
}
