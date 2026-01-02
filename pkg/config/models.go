package config

import "time"

type Config struct {
	Logging            Logging             `yaml:"logging"`
	Dataplane          Dataplane           `yaml:"dataplane"`
	Redis              Redis               `yaml:"redis"`
	Redundancy         Redundancy          `yaml:"redundancy"`
	SubscriberGateways []SubscriberGateway `yaml:"subscriber_gateways"`
	SubscriberGroup    SubscriberGroup     `yaml:"subscriber_group"`
	AAA                AAA                 `yaml:"aaa"`
	DHCP               DHCP                `yaml:"dhcp"`
	ZebraSocket        string              `yaml:"zebra_socket,omitempty"`
	Monitoring         Monitoring          `yaml:"monitoring,omitempty"`
	API                API                 `yaml:"api,omitempty"`
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
	CoreInterface     string `yaml:"core_interface"`
	CPEgressInterface string `yaml:"cp_egress_interface,omitempty"`
	VPPAPISocket      string `yaml:"vpp_api_socket,omitempty"`
	LinuxInterface    string `yaml:"linux_interface,omitempty"`
	PuntSocketPath    string `yaml:"punt_socket_path,omitempty"`
	ARPPuntSocketPath string `yaml:"arp_punt_socket_path,omitempty"`
	MemifSocketPath   string `yaml:"memif_socket_path,omitempty"`
	GatewayMAC        string `yaml:"gateway_mac,omitempty"`
}

type Redis struct {
	Address  string `yaml:"address"`
	Password string `yaml:"password"`
}

type Redundancy struct {
	VirtualMAC        string        `yaml:"virtual_mac"`
	BNGID             string        `yaml:"bng_id"`
	Priority          int           `yaml:"priority"`
	HeartbeatInterval time.Duration `yaml:"heartbeat_interval"`
	MemberTimeout     time.Duration `yaml:"member_timeout"`
}

type SubscriberGateway struct {
	Name string   `yaml:"name"`
	IPv4 []string `yaml:"ipv4"`
	IPv6 []string `yaml:"ipv6,omitempty"`
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

type VLANAAAs struct {
	Enabled bool   `yaml:"enabled"`
	RADIUS  string `yaml:"radius"`
	Policy  string `yaml:"policy"`
}

type AAA struct {
	Provider      string        `yaml:"provider"`
	NASIdentifier string        `yaml:"nas_identifier"`
	NASIP         string        `yaml:"nas_ip"`
	Policy        []AAAPolicy   `yaml:"policy"`
	RADIUS        []RADIUSGroup `yaml:"radius"`
}

type AAAPolicy struct {
	Name                  string `yaml:"name"`
	Format                string `yaml:"format"`
	Type                  string `yaml:"type"`
	MaxConcurrentSessions int    `yaml:"max_concurrent_sessions"`
}

type RADIUSGroup struct {
	Name               string         `yaml:"name"`
	Enabled            bool           `yaml:"enabled"`
	Servers            []RADIUSServer `yaml:"servers"`
	AccountingInterval int            `yaml:"accounting_interval"`
	SourceAddress      string         `yaml:"source_address"`
}

type RADIUSServer struct {
	Address string `yaml:"address"`
	Port    int    `yaml:"port"`
	Secret  string `yaml:"secret"`
	Type    string `yaml:"type"`
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
	Prometheus        PrometheusConfig `yaml:"prometheus,omitempty"`
	EnabledCollectors []string         `yaml:"enabled_collectors,omitempty"`
}

type PrometheusConfig struct {
	Enabled        bool          `yaml:"enabled"`
	ListenAddress  string        `yaml:"listen_address"`
	CollectInterval time.Duration `yaml:"collect_interval,omitempty"`
	StateTTL       time.Duration `yaml:"state_ttl,omitempty"`
}
