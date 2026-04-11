package interfaces

import (
	"encoding/json"
	"fmt"
	"net"
)

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
	LCP     bool        `json:"-" yaml:"-"`
	VRF     string      `json:"vrf,omitempty" yaml:"vrf,omitempty"`
	BNGMode string      `json:"bng_mode,omitempty" yaml:"bng_mode,omitempty"`
	CGNAT   string      `json:"cgnat,omitempty" yaml:"cgnat,omitempty"`

	Subinterfaces SubinterfaceMap `json:"subinterfaces,omitempty" yaml:"subinterfaces,omitempty"`
	IPv6          *IPv6Config                    `json:"ipv6,omitempty" yaml:"ipv6,omitempty"`
	ARP           *ARPConfig                     `json:"arp,omitempty" yaml:"arp,omitempty"`
	Unnumbered    string                         `json:"unnumbered,omitempty" yaml:"unnumbered,omitempty"`
}

type SubinterfaceConfig struct {
	// Creation fields (handled by SubinterfaceHandler, not walked)
	ID           int    `json:"id" yaml:"id"`
	VLAN         int    `json:"vlan" yaml:"vlan"`
	InnerVLAN    *int   `json:"inner-vlan,omitempty" yaml:"inner-vlan,omitempty"`
	VLANTpid     string `json:"vlan-tpid,omitempty" yaml:"vlan-tpid,omitempty"`

	// Per-property fields (walked in struct order by confmgr)
	LCP         bool           `json:"-" yaml:"-"`
	VRF         string         `json:"vrf,omitempty" yaml:"vrf,omitempty"`
	MTU         int            `json:"mtu,omitempty" yaml:"mtu,omitempty"`
	Description string         `json:"description,omitempty" yaml:"description,omitempty"`
	Address     *AddressConfig `json:"address,omitempty" yaml:"address,omitempty"`
	IPv6        *IPv6Config    `json:"ipv6,omitempty" yaml:"ipv6,omitempty"`
	ARP         *ARPConfig     `json:"arp,omitempty" yaml:"arp,omitempty"`
	Unnumbered  string         `json:"unnumbered,omitempty" yaml:"unnumbered,omitempty"`
	BNG         *BNGConfig     `json:"bng,omitempty" yaml:"bng,omitempty"`
	MSSClamp    *MSSClampSpec  `json:"-" yaml:"-"`
	Enabled     bool           `json:"enabled,omitempty" yaml:"enabled,omitempty"`
}

type MSSClampSpec struct {
	Enabled bool
	IPv4MSS uint16
	IPv6MSS uint16
}

func (c *InterfaceConfig) NeedsLCP() bool {
	// Reserved in case we need to build more complicated logic
	// to automatically determine LCP being auto-enabled
	// 99% of scenarios, most operators will want routing/arp/nd on
	// all core interfaces
	return true
}

func (c *SubinterfaceConfig) NeedsLCP() bool {
	return c.BNG == nil
}

type SubinterfaceMap map[string]*SubinterfaceConfig

func (m *SubinterfaceMap) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var list []*SubinterfaceConfig
	if err := unmarshal(&list); err == nil {
		*m = make(SubinterfaceMap, len(list))
		for _, sub := range list {
			key := fmt.Sprintf("%d", sub.ID)
			(*m)[key] = sub
		}
		return nil
	}

	var raw map[string]*SubinterfaceConfig
	if err := unmarshal(&raw); err != nil {
		return err
	}
	*m = SubinterfaceMap(raw)
	return nil
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
	Mode        string       `json:"mode" yaml:"mode"`
	Members     []BondMember `json:"members" yaml:"members"`
	LoadBalance string       `json:"load-balance,omitempty" yaml:"load-balance,omitempty"`
	GSO         bool         `json:"gso,omitempty" yaml:"gso,omitempty"`
	MACAddress  string       `json:"mac-address,omitempty" yaml:"mac-address,omitempty"`
}

func (c *BondConfig) Validate() error {
	switch c.Mode {
	case "lacp", "round-robin", "active-backup", "xor", "broadcast", "":
	default:
		return fmt.Errorf("invalid bond mode %q", c.Mode)
	}

	if len(c.Members) == 0 {
		return fmt.Errorf("bond requires at least one member")
	}

	if c.LoadBalance != "" {
		switch c.LoadBalance {
		case "l2", "l23", "l34":
		default:
			return fmt.Errorf("invalid load-balance %q (must be l2, l23, or l34)", c.LoadBalance)
		}
		mode := c.Mode
		if mode == "" {
			mode = "lacp"
		}
		if mode != "lacp" && mode != "xor" {
			return fmt.Errorf("load-balance is only valid for lacp and xor modes")
		}
	}

	if c.MACAddress != "" {
		if _, err := net.ParseMAC(c.MACAddress); err != nil {
			return fmt.Errorf("invalid mac-address %q: %w", c.MACAddress, err)
		}
	}

	return nil
}

type BondMember struct {
	Name        string `json:"name" yaml:"name"`
	Passive     bool   `json:"passive,omitempty" yaml:"passive,omitempty"`
	LongTimeout bool   `json:"long-timeout,omitempty" yaml:"long-timeout,omitempty"`
}

func (m *BondMember) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		m.Name = s
		return nil
	}

	type plain BondMember
	return json.Unmarshal(data, (*plain)(m))
}

func (m *BondMember) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err == nil {
		m.Name = s
		return nil
	}

	type plain BondMember
	return unmarshal((*plain)(m))
}
