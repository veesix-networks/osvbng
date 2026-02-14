package interfaces

type InterfaceConfig struct {
	Name        string         `json:"name" yaml:"name"`
	Description string         `json:"description,omitempty" yaml:"description,omitempty"`
	Enabled     bool           `json:"enabled" yaml:"enabled"`
	MTU         int            `json:"mtu,omitempty" yaml:"mtu,omitempty"`
	Address     *AddressConfig `json:"address,omitempty" yaml:"address,omitempty"`

	Type    string      `json:"type,omitempty" yaml:"type,omitempty"`
	Parent  string      `json:"parent,omitempty" yaml:"parent,omitempty"`
	VLANID  int         `json:"vlan-id,omitempty" yaml:"vlan-id,omitempty"`
	Bond    *BondConfig `json:"bond,omitempty" yaml:"bond,omitempty"`
	LCP     bool        `json:"lcp,omitempty" yaml:"lcp,omitempty"`
	VRF     string      `json:"vrf,omitempty" yaml:"vrf,omitempty"`
	BNGMode string      `json:"bng_mode,omitempty" yaml:"bng_mode,omitempty"`

	Subinterfaces map[string]*SubinterfaceConfig `json:"subinterfaces,omitempty" yaml:"subinterfaces,omitempty"`
	IPv6          *IPv6Config                    `json:"ipv6,omitempty" yaml:"ipv6,omitempty"`
	ARP           *ARPConfig                     `json:"arp,omitempty" yaml:"arp,omitempty"`
	Unnumbered    string                         `json:"unnumbered,omitempty" yaml:"unnumbered,omitempty"`
}

type SubinterfaceConfig struct {
	VLAN       int         `json:"vlan" yaml:"vlan"`
	Enabled    bool        `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	IPv6       *IPv6Config `json:"ipv6,omitempty" yaml:"ipv6,omitempty"`
	ARP        *ARPConfig  `json:"arp,omitempty" yaml:"arp,omitempty"`
	Unnumbered string      `json:"unnumbered,omitempty" yaml:"unnumbered,omitempty"`
	BNG        *BNGConfig  `json:"bng,omitempty" yaml:"bng,omitempty"`
}

type BNGMode string

const (
	BNGModeIPoE   BNGMode = "ipoe"    // IPoE: DHCPv4, DHCPv6, ARP
	BNGModeIPoEL3 BNGMode = "ipoe-l3" // IPoE L3: DHCP relay
	BNGModePPPoE  BNGMode = "pppoe"   // PPPoE local termination
	BNGModeLAC    BNGMode = "lac"     // PPPoE -> L2TP tunnel to LNS
	BNGModeLNS    BNGMode = "lns"     // L2TP tunnel termination
)

type BNGConfig struct {
	Mode BNGMode `json:"mode" yaml:"mode"`
}

type IPv6Config struct {
	Enabled   bool       `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	RA        *RAConfig  `json:"ra,omitempty" yaml:"ra,omitempty"`
	Multicast bool       `json:"multicast,omitempty" yaml:"multicast,omitempty"`
}

type RAConfig struct {
	Managed        bool   `json:"managed,omitempty" yaml:"managed,omitempty"`
	Other          bool   `json:"other,omitempty" yaml:"other,omitempty"`
	RouterLifetime uint32 `json:"router-lifetime,omitempty" yaml:"router-lifetime,omitempty"`
	MaxInterval    uint32 `json:"max-interval,omitempty" yaml:"max-interval,omitempty"`
	MinInterval    uint32 `json:"min-interval,omitempty" yaml:"min-interval,omitempty"`
}

type ARPConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
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
