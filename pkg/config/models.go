package config

import "time"

type Config struct {
	Logging         Logging                           `yaml:"logging,omitempty"`
	Dataplane       Dataplane                         `yaml:"dataplane,omitempty"`
	SubscriberGroup SubscriberGroup                   `yaml:"subscriber_group,omitempty"`
	AAA             AAA                               `yaml:"aaa,omitempty"`
	DHCP            DHCP                              `yaml:"dhcp,omitempty"`
	ZebraSocket     string                            `yaml:"zebra_socket,omitempty"`
	Monitoring      Monitoring                        `yaml:"monitoring,omitempty"`
	API             API                               `yaml:"api,omitempty"`
	Plugins         map[string]map[string]interface{} `yaml:"plugins,omitempty"`
}

type API struct {
	Address string `yaml:"address"`
}

type Logging struct {
	Format     string            `yaml:"format"`
	Level      string            `yaml:"level"`
	Components map[string]string `yaml:"components,omitempty"`
}

type Dataplane struct {
	AccessInterface   string `yaml:"access_interface"`
	DPAPISocket       string `yaml:"dp_api_socket,omitempty"`
	PuntSocketPath    string `yaml:"punt_socket_path,omitempty"`
	ARPPuntSocketPath string `yaml:"arp_punt_socket_path,omitempty"`
	MemifSocketPath   string `yaml:"memif_socket_path,omitempty"`
	DPDK              *DPDK  `yaml:"dpdk,omitempty"`
}

type DPDK struct {
	UIODriver            string             `yaml:"uio_driver,omitempty"`
	Devices              []DPDKDevice       `yaml:"devices,omitempty"`
	DevDefaults          *DPDKDeviceOptions `yaml:"dev_defaults,omitempty"`
	SocketMem            string             `yaml:"socket_mem,omitempty"`
	NoMultiSeg           bool               `yaml:"no_multi_seg,omitempty"`
	NoTxChecksumOffload  bool               `yaml:"no_tx_checksum_offload,omitempty"`
	EnableTcpUdpChecksum bool               `yaml:"enable_tcp_udp_checksum,omitempty"`
	MaxSimdBitwidth      int                `yaml:"max_simd_bitwidth,omitempty"`
}

type DPDKDevice struct {
	PCI     string             `yaml:"pci"`
	Name    string             `yaml:"name,omitempty"`
	Options *DPDKDeviceOptions `yaml:"options,omitempty"`
}

type DPDKDeviceOptions struct {
	NumRxQueues   int    `yaml:"num_rx_queues,omitempty"`
	NumTxQueues   int    `yaml:"num_tx_queues,omitempty"`
	NumRxDesc     int    `yaml:"num_rx_desc,omitempty"`
	NumTxDesc     int    `yaml:"num_tx_desc,omitempty"`
	TSO           bool   `yaml:"tso,omitempty"`
	Devargs       string `yaml:"devargs,omitempty"`
	RssQueues     string `yaml:"rss_queues,omitempty"`
	NoRxInterrupt bool   `yaml:"no_rx_interrupt,omitempty"`
}

type Redundancy struct {
	VirtualMAC        string        `yaml:"virtual_mac"`
	BNGID             string        `yaml:"bng_id"`
	Priority          int           `yaml:"priority"`
	HeartbeatInterval time.Duration `yaml:"heartbeat_interval"`
	MemberTimeout     time.Duration `yaml:"member_timeout"`
}

type SubscriberGroup struct {
	DefaultPolicy string      `yaml:"default_policy"`
	VLANs         []VLANRange `yaml:"vlans"`
}

type VLANRange struct {
	SVLAN     string    `yaml:"svlan"`
	CVLAN     string    `yaml:"cvlan"`
	Interface string    `yaml:"interface"`
	IPv4      []string  `yaml:"ipv4,omitempty"`
	IPv6      []string  `yaml:"ipv6,omitempty"`
	DHCP      string    `yaml:"dhcp,omitempty"`
	AAA       *VLANAAAs `yaml:"aaa,omitempty"`
	Template  string    `yaml:"template,omitempty"`
}

func (v *VLANRange) GetSVLANs() ([]uint16, error) {
	return ParseVLANRange(v.SVLAN)
}

func (v *VLANRange) GetCVLAN() (isAny bool, cvlan uint16, err error) {
	return ParseCVLAN(v.CVLAN)
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

func (sg *SubscriberGroup) FindGatewayForSVLAN(svlan uint16) string {
	for _, vlanCfg := range sg.VLANs {
		if vlanCfg.MatchesSVLAN(svlan) {
			return vlanCfg.Interface
		}
	}
	return ""
}

func (sg *SubscriberGroup) FindVLANConfig(svlan uint16) *VLANRange {
	for i := range sg.VLANs {
		if sg.VLANs[i].MatchesSVLAN(svlan) {
			return &sg.VLANs[i]
		}
	}
	return nil
}

func (sg *SubscriberGroup) GetPolicyName(svlan uint16) string {
	vlanCfg := sg.FindVLANConfig(svlan)
	if vlanCfg != nil && vlanCfg.AAA != nil && vlanCfg.AAA.Policy != "" {
		return vlanCfg.AAA.Policy
	}
	return sg.DefaultPolicy
}

type VLANAAAs struct {
	Enabled bool   `yaml:"enabled"`
	Policy  string `yaml:"policy"`
}

type AAA struct {
	Provider      string      `yaml:"provider"`
	NASIdentifier string      `yaml:"nas_identifier"`
	NASIP         string      `yaml:"nas_ip"`
	Policy        []AAAPolicy `yaml:"policy"`
}

type AAAPolicy struct {
	Name                  string `yaml:"name"`
	Format                string `yaml:"format"`
	Type                  string `yaml:"type"`
	MaxConcurrentSessions int    `yaml:"max_concurrent_sessions"`
}

type DHCP struct {
	Provider      string       `yaml:"provider"`
	DefaultServer string       `yaml:"default_server"`
	Servers       []DHCPServer `yaml:"servers"`
	Pools         []DHCPPool   `yaml:"pools"`
}

type DHCPServer struct {
	Name    string `yaml:"name"`
	Address string `yaml:"address"`
	GIAddr  string `yaml:"giaddr"`
}

type DHCPPool struct {
	Name       string   `yaml:"name"`
	Network    string   `yaml:"network"`
	RangeStart string   `yaml:"range_start"`
	RangeEnd   string   `yaml:"range_end"`
	Gateway    string   `yaml:"gateway"`
	DNSServers []string `yaml:"dns_servers"`
	LeaseTime  uint32   `yaml:"lease_time"`
}

type Monitoring struct {
	DisabledCollectors []string `yaml:"disabled_collectors,omitempty"`
	CollectInterval   time.Duration `yaml:"collect_interval,omitempty"`
}
