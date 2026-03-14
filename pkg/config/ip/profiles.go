package ip

import "time"

type IPv4Pool struct {
	Name       string   `json:"name,omitempty" yaml:"name,omitempty"`
	Network    string   `json:"network,omitempty" yaml:"network,omitempty"`
	RangeStart string   `json:"range_start,omitempty" yaml:"range-start,omitempty"`
	RangeEnd   string   `json:"range_end,omitempty" yaml:"range-end,omitempty"`
	Gateway    string   `json:"gateway,omitempty" yaml:"gateway,omitempty"`
	VRF        string   `json:"vrf,omitempty" yaml:"vrf,omitempty"`
	DNSServers []string `json:"dns_servers,omitempty" yaml:"dns,omitempty"`
	LeaseTime  uint32   `json:"lease_time,omitempty" yaml:"lease-time,omitempty"`
	Priority   int      `json:"priority,omitempty" yaml:"priority,omitempty"`
	Exclude    []string `json:"exclude,omitempty" yaml:"exclude,omitempty"`
}

type IANAPool struct {
	Name          string `json:"name,omitempty" yaml:"name,omitempty"`
	Network       string `json:"network,omitempty" yaml:"network,omitempty"`
	RangeStart    string `json:"range_start,omitempty" yaml:"range_start,omitempty"`
	RangeEnd      string `json:"range_end,omitempty" yaml:"range_end,omitempty"`
	Gateway       string `json:"gateway,omitempty" yaml:"gateway,omitempty"`
	VRF           string `json:"vrf,omitempty" yaml:"vrf,omitempty"`
	PreferredTime uint32 `json:"preferred_time,omitempty" yaml:"preferred_time,omitempty"`
	ValidTime     uint32 `json:"valid_time,omitempty" yaml:"valid_time,omitempty"`
}

type PDPool struct {
	Name          string `json:"name,omitempty" yaml:"name,omitempty"`
	Network       string `json:"network,omitempty" yaml:"network,omitempty"`
	PrefixLength  uint8  `json:"prefix_length,omitempty" yaml:"prefix_length,omitempty"`
	VRF           string `json:"vrf,omitempty" yaml:"vrf,omitempty"`
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

type DHCPRelayServer struct {
	Address  string `json:"address" yaml:"address"`
	Priority int    `json:"priority,omitempty" yaml:"priority,omitempty"`
}

type Option82Config struct {
	CircuitIDFormat string `json:"circuit_id_format,omitempty" yaml:"circuit-id-format,omitempty"`
	RemoteIDFormat  string `json:"remote_id_format,omitempty" yaml:"remote-id-format,omitempty"`
	Policy          string `json:"policy,omitempty" yaml:"policy,omitempty"`
	IncludeFlags    bool   `json:"include_flags,omitempty" yaml:"include-flags,omitempty"`
}

type LeasequeryConfig struct {
	Enabled        bool          `json:"enabled" yaml:"enabled"`
	PollInterval   time.Duration `json:"poll_interval,omitempty" yaml:"poll-interval,omitempty"`
	OnDemand       bool          `json:"on_demand,omitempty" yaml:"on-demand,omitempty"`
	RebootRecovery bool          `json:"reboot_recovery,omitempty" yaml:"reboot-recovery,omitempty"`
}

type IPv4DHCPOptions struct {
	Mode            string            `json:"mode,omitempty" yaml:"mode,omitempty"`
	AddressModel    string            `json:"address_model,omitempty" yaml:"address-model,omitempty"`
	ServerID        string            `json:"server_id,omitempty" yaml:"server-id,omitempty"`
	LeaseTime       uint32            `json:"lease_time,omitempty" yaml:"lease-time,omitempty"`
	Servers         []DHCPRelayServer `json:"servers,omitempty" yaml:"servers,omitempty"`
	GIAddr          string            `json:"giaddr,omitempty" yaml:"giaddr,omitempty"`
	ServerTimeout   time.Duration     `json:"server_timeout,omitempty" yaml:"server-timeout,omitempty"`
	GraceMultiplier float64           `json:"grace_multiplier,omitempty" yaml:"grace-multiplier,omitempty"`
	ClientLease     uint32            `json:"client_lease,omitempty" yaml:"client-lease,omitempty"`
	Option82        *Option82Config   `json:"option82,omitempty" yaml:"option82,omitempty"`
	Leasequery      *LeasequeryConfig `json:"leasequery,omitempty" yaml:"leasequery,omitempty"`
	DeadTime        time.Duration     `json:"dead_time,omitempty" yaml:"dead-time,omitempty"`
	DeadThreshold   int               `json:"dead_threshold,omitempty" yaml:"dead-threshold,omitempty"`
}

type IPv4ICPPOptions struct{}

type IPv6DHCPv6Options struct {
	Mode                    string            `json:"mode,omitempty" yaml:"mode,omitempty"`
	PreferredTime           uint32            `json:"preferred_time,omitempty" yaml:"preferred-time,omitempty"`
	ValidTime               uint32            `json:"valid_time,omitempty" yaml:"valid-time,omitempty"`
	Servers                 []DHCPRelayServer `json:"servers,omitempty" yaml:"servers,omitempty"`
	LinkAddress             string            `json:"link_address,omitempty" yaml:"link-address,omitempty"`
	ServerTimeout           time.Duration     `json:"server_timeout,omitempty" yaml:"server-timeout,omitempty"`
	GraceMultiplier         float64           `json:"grace_multiplier,omitempty" yaml:"grace-multiplier,omitempty"`
	ClientPreferredLifetime uint32            `json:"client_preferred_lifetime,omitempty" yaml:"client-preferred-lifetime,omitempty"`
	ClientValidLifetime     uint32            `json:"client_valid_lifetime,omitempty" yaml:"client-valid-lifetime,omitempty"`
	InterfaceIDFormat       string            `json:"interface_id_format,omitempty" yaml:"interface-id-format,omitempty"`
	RemoteIDFormat          string            `json:"remote_id_format,omitempty" yaml:"remote-id-format,omitempty"`
	EnterpriseNumber        uint32            `json:"enterprise_number,omitempty" yaml:"enterprise-number,omitempty"`
	SubscriberIDFormat      string            `json:"subscriber_id_format,omitempty" yaml:"subscriber-id-format,omitempty"`
	Leasequery              *LeasequeryConfig `json:"leasequery,omitempty" yaml:"leasequery,omitempty"`
	DeadTime                time.Duration     `json:"dead_time,omitempty" yaml:"dead-time,omitempty"`
	DeadThreshold           int               `json:"dead_threshold,omitempty" yaml:"dead-threshold,omitempty"`
}

type IPv6CPOptions struct{}

type IPv4Profile struct {
	Gateway string           `json:"gateway,omitempty" yaml:"gateway,omitempty"`
	VRF     string           `json:"vrf,omitempty" yaml:"vrf,omitempty"`
	Pools   []IPv4Pool       `json:"pools,omitempty" yaml:"pools,omitempty"`
	DNS     []string         `json:"dns,omitempty" yaml:"dns,omitempty"`
	DHCP    *IPv4DHCPOptions `json:"dhcp,omitempty" yaml:"dhcp,omitempty"`
	IPCP    *IPv4ICPPOptions `json:"ipcp,omitempty" yaml:"ipcp,omitempty"`
}

func (p *IPv4Profile) GetMode() string {
	if p == nil || p.DHCP == nil || p.DHCP.Mode == "" {
		return "server"
	}
	return p.DHCP.Mode
}

func (p *IPv4Profile) GetAddressModel() string {
	if p == nil || p.DHCP == nil || p.DHCP.AddressModel == "" {
		return "connected-subnet"
	}
	return p.DHCP.AddressModel
}

func (p *IPv4Profile) GetLeaseTime() uint32 {
	if p == nil || p.DHCP == nil || p.DHCP.LeaseTime == 0 {
		return 3600
	}
	return p.DHCP.LeaseTime
}

func (p *IPv4Profile) GetServerID() string {
	if p == nil || p.DHCP == nil {
		return ""
	}
	return p.DHCP.ServerID
}

func (p *IPv4Profile) GetServerTimeout() time.Duration {
	if p == nil || p.DHCP == nil || p.DHCP.ServerTimeout == 0 {
		return 5 * time.Second
	}
	return p.DHCP.ServerTimeout
}

func (p *IPv4Profile) GetGraceMultiplier() float64 {
	if p == nil || p.DHCP == nil || p.DHCP.GraceMultiplier == 0 {
		return 2.0
	}
	return p.DHCP.GraceMultiplier
}

func (p *IPv4Profile) GetClientLease() uint32 {
	if p == nil || p.DHCP == nil || p.DHCP.ClientLease == 0 {
		return 300
	}
	return p.DHCP.ClientLease
}

func (p *IPv4Profile) GetDeadTime() time.Duration {
	if p == nil || p.DHCP == nil || p.DHCP.DeadTime == 0 {
		return 30 * time.Second
	}
	return p.DHCP.DeadTime
}

func (p *IPv4Profile) GetDeadThreshold() int {
	if p == nil || p.DHCP == nil || p.DHCP.DeadThreshold == 0 {
		return 3
	}
	return p.DHCP.DeadThreshold
}

func (o *Option82Config) GetPolicy() string {
	if o == nil || o.Policy == "" {
		return "replace"
	}
	return o.Policy
}

func (o *Option82Config) GetCircuitIDFormat() string {
	if o == nil || o.CircuitIDFormat == "" {
		return "{interface}:{svlan}:{cvlan}"
	}
	return o.CircuitIDFormat
}

func (o *Option82Config) GetRemoteIDFormat() string {
	if o == nil || o.RemoteIDFormat == "" {
		return "{mac}"
	}
	return o.RemoteIDFormat
}

func (l *LeasequeryConfig) GetPollInterval() time.Duration {
	if l == nil || l.PollInterval == 0 {
		return 60 * time.Second
	}
	return l.PollInterval
}

type IPv6Profile struct {
	VRF       string             `json:"vrf,omitempty" yaml:"vrf,omitempty"`
	IANAPools []IANAPool         `json:"iana_pools,omitempty" yaml:"iana-pools,omitempty"`
	PDPools   []PDPool           `json:"pd_pools,omitempty" yaml:"pd-pools,omitempty"`
	DNS       []string           `json:"dns,omitempty" yaml:"dns,omitempty"`
	RA        *IPv6RAConfig      `json:"ra,omitempty" yaml:"ra,omitempty"`
	DHCPv6    *IPv6DHCPv6Options `json:"dhcpv6,omitempty" yaml:"dhcpv6,omitempty"`
	IPv6CP    *IPv6CPOptions     `json:"ipv6cp,omitempty" yaml:"ipv6cp,omitempty"`
}

func (p *IPv6Profile) GetMode() string {
	if p == nil || p.DHCPv6 == nil || p.DHCPv6.Mode == "" {
		return "server"
	}
	return p.DHCPv6.Mode
}

func (p *IPv6Profile) GetPreferredTime() uint32 {
	if p == nil || p.DHCPv6 == nil || p.DHCPv6.PreferredTime == 0 {
		return 3600
	}
	return p.DHCPv6.PreferredTime
}

func (p *IPv6Profile) GetValidTime() uint32 {
	if p == nil || p.DHCPv6 == nil || p.DHCPv6.ValidTime == 0 {
		return 7200
	}
	return p.DHCPv6.ValidTime
}

func (p *IPv6Profile) GetServerTimeout() time.Duration {
	if p == nil || p.DHCPv6 == nil || p.DHCPv6.ServerTimeout == 0 {
		return 5 * time.Second
	}
	return p.DHCPv6.ServerTimeout
}

func (p *IPv6Profile) GetGraceMultiplier() float64 {
	if p == nil || p.DHCPv6 == nil || p.DHCPv6.GraceMultiplier == 0 {
		return 2.0
	}
	return p.DHCPv6.GraceMultiplier
}

func (p *IPv6Profile) GetClientPreferredLifetime() uint32 {
	if p == nil || p.DHCPv6 == nil || p.DHCPv6.ClientPreferredLifetime == 0 {
		return 300
	}
	return p.DHCPv6.ClientPreferredLifetime
}

func (p *IPv6Profile) GetClientValidLifetime() uint32 {
	if p == nil || p.DHCPv6 == nil || p.DHCPv6.ClientValidLifetime == 0 {
		return 300
	}
	return p.DHCPv6.ClientValidLifetime
}

func (p *IPv6Profile) GetDeadTime() time.Duration {
	if p == nil || p.DHCPv6 == nil || p.DHCPv6.DeadTime == 0 {
		return 30 * time.Second
	}
	return p.DHCPv6.DeadTime
}

func (p *IPv6Profile) GetDeadThreshold() int {
	if p == nil || p.DHCPv6 == nil || p.DHCPv6.DeadThreshold == 0 {
		return 3
	}
	return p.DHCPv6.DeadThreshold
}
