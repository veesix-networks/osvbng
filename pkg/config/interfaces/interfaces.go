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
	BNGMode string      `json:"bng_mode,omitempty" yaml:"bng_mode,omitempty"`
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
