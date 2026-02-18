package ip

type DHCPConfig struct {
	Provider      string                  `json:"provider,omitempty" yaml:"provider,omitempty"`
	DefaultServer string                  `json:"default_server,omitempty" yaml:"default_server,omitempty"`
	Servers       []DHCPServer            `json:"servers,omitempty" yaml:"servers,omitempty"`
	Pools         []DHCPPool              `json:"pools,omitempty" yaml:"pools,omitempty"`
	Profiles      map[string]*DHCPProfile `json:"profiles,omitempty" yaml:"profiles,omitempty"`
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
	RangeStart string   `json:"range_start,omitempty" yaml:"range-start,omitempty"`
	RangeEnd   string   `json:"range_end,omitempty" yaml:"range-end,omitempty"`
	Gateway    string   `json:"gateway,omitempty" yaml:"gateway,omitempty"`
	DNSServers []string `json:"dns_servers,omitempty" yaml:"dns,omitempty"`
	LeaseTime  uint32   `json:"lease_time,omitempty" yaml:"lease-time,omitempty"`
	Priority   int      `json:"priority,omitempty" yaml:"priority,omitempty"`
}

type DHCPv6Config struct {
	Provider   string                    `json:"provider,omitempty" yaml:"provider,omitempty"`
	IANAPools  []DHCPv6Pool              `json:"iana_pools,omitempty" yaml:"iana_pools,omitempty"`
	PDPools    []DHCPv6PDPool            `json:"pd_pools,omitempty" yaml:"pd_pools,omitempty"`
	DNSServers []string                  `json:"dns_servers,omitempty" yaml:"dns_servers,omitempty"`
	DomainList []string                  `json:"domain_list,omitempty" yaml:"domain_list,omitempty"`
	RA         *IPv6RAConfig             `json:"ra,omitempty" yaml:"ra,omitempty"`
	Profiles   map[string]*DHCPv6Profile `json:"profiles,omitempty" yaml:"profiles,omitempty"`
}

type DHCPv6Pool struct {
	Name          string `json:"name,omitempty" yaml:"name,omitempty"`
	Network       string `json:"network,omitempty" yaml:"network,omitempty"`
	RangeStart    string `json:"range_start,omitempty" yaml:"range_start,omitempty"`
	RangeEnd      string `json:"range_end,omitempty" yaml:"range_end,omitempty"`
	Gateway       string `json:"gateway,omitempty" yaml:"gateway,omitempty"`
	PreferredTime uint32 `json:"preferred_time,omitempty" yaml:"preferred_time,omitempty"`
	ValidTime     uint32 `json:"valid_time,omitempty" yaml:"valid_time,omitempty"`
}

type DHCPv6PDPool struct {
	Name          string `json:"name,omitempty" yaml:"name,omitempty"`
	Network       string `json:"network,omitempty" yaml:"network,omitempty"`
	PrefixLength  uint8  `json:"prefix_length,omitempty" yaml:"prefix_length,omitempty"`
	PreferredTime uint32 `json:"preferred_time,omitempty" yaml:"preferred_time,omitempty"`
	ValidTime     uint32 `json:"valid_time,omitempty" yaml:"valid_time,omitempty"`
}

type IPv6RAConfig struct {
	Managed        *bool  `json:"managed,omitempty" yaml:"managed,omitempty"`
	Other          *bool  `json:"other,omitempty" yaml:"other,omitempty"`
	RouterLifetime uint32 `json:"router_lifetime,omitempty" yaml:"router_lifetime,omitempty"`
	MaxInterval    uint32 `json:"max_interval,omitempty" yaml:"max_interval,omitempty"`
	MinInterval    uint32 `json:"min_interval,omitempty" yaml:"min_interval,omitempty"`
}

func (r *IPv6RAConfig) GetManaged() bool {
	if r == nil || r.Managed == nil {
		return true
	}
	return *r.Managed
}

func (r *IPv6RAConfig) GetOther() bool {
	if r == nil || r.Other == nil {
		return true
	}
	return *r.Other
}

func (r *IPv6RAConfig) GetRouterLifetime() uint32 {
	if r == nil || r.RouterLifetime == 0 {
		return 1800
	}
	return r.RouterLifetime
}

func (r *IPv6RAConfig) GetMaxInterval() uint32 {
	if r == nil || r.MaxInterval == 0 {
		return 600
	}
	return r.MaxInterval
}

func (r *IPv6RAConfig) GetMinInterval() uint32 {
	if r == nil || r.MinInterval == 0 {
		return 200
	}
	return r.MinInterval
}

type DHCPProfile struct {
	Mode         string     `json:"mode,omitempty" yaml:"mode,omitempty"`
	AddressModel string     `json:"address_model,omitempty" yaml:"address-model,omitempty"`
	Gateway      string     `json:"gateway,omitempty" yaml:"gateway,omitempty"`
	ServerID     string     `json:"server_id,omitempty" yaml:"server-id,omitempty"`
	VRF          string     `json:"vrf,omitempty" yaml:"vrf,omitempty"`
	Pools        []DHCPPool `json:"pools,omitempty" yaml:"pools,omitempty"`
	DNS          []string   `json:"dns,omitempty" yaml:"dns,omitempty"`
	LeaseTime    uint32     `json:"lease_time,omitempty" yaml:"lease-time,omitempty"`
}

func (p *DHCPProfile) GetMode() string {
	if p == nil || p.Mode == "" {
		return "server"
	}
	return p.Mode
}

func (p *DHCPProfile) GetAddressModel() string {
	if p == nil || p.AddressModel == "" {
		return "connected-subnet"
	}
	return p.AddressModel
}

func (p *DHCPProfile) GetLeaseTime() uint32 {
	if p == nil || p.LeaseTime == 0 {
		return 3600
	}
	return p.LeaseTime
}

type DHCPv6Profile struct {
	Mode          string         `json:"mode,omitempty" yaml:"mode,omitempty"`
	VRF           string         `json:"vrf,omitempty" yaml:"vrf,omitempty"`
	IANAPools     []DHCPv6Pool   `json:"iana_pools,omitempty" yaml:"iana-pools,omitempty"`
	PDPools       []DHCPv6PDPool `json:"pd_pools,omitempty" yaml:"pd-pools,omitempty"`
	DNS           []string       `json:"dns,omitempty" yaml:"dns,omitempty"`
	PreferredTime uint32         `json:"preferred_time,omitempty" yaml:"preferred-time,omitempty"`
	ValidTime     uint32         `json:"valid_time,omitempty" yaml:"valid-time,omitempty"`
	RA            *IPv6RAConfig  `json:"ra,omitempty" yaml:"ra,omitempty"`
}

func (p *DHCPv6Profile) GetMode() string {
	if p == nil || p.Mode == "" {
		return "server"
	}
	return p.Mode
}

func (p *DHCPv6Profile) GetPreferredTime() uint32 {
	if p == nil || p.PreferredTime == 0 {
		return 3600
	}
	return p.PreferredTime
}

func (p *DHCPv6Profile) GetValidTime() uint32 {
	if p == nil || p.ValidTime == 0 {
		return 7200
	}
	return p.ValidTime
}
