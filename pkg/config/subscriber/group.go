package subscriber

import (
	"github.com/veesix-networks/osvbng/pkg/config/vlan"
)

type SubscriberGroupsConfig struct {
	Groups map[string]*SubscriberGroup `json:"groups,omitempty" yaml:"groups,omitempty"`
}

func (sgc *SubscriberGroupsConfig) FindGroupBySVLAN(svlan uint16) (*SubscriberGroup, *VLANRange) {
	for _, group := range sgc.Groups {
		for i := range group.VLANs {
			if group.VLANs[i].MatchesSVLAN(svlan) {
				return group, &group.VLANs[i]
			}
		}
	}
	return nil, nil
}

// SessionMode determines how IPv4 and IPv6 sessions are managed
type SessionMode string

const (
	// SessionModeUnified uses a single session per MAC+VLAN for both IPv4 and IPv6.
	// First protocol triggers AAA, second finds existing approved session.
	SessionModeUnified SessionMode = "unified"

	// SessionModeIndependent uses separate sessions for IPv4 and IPv6.
	// Each triggers its own AAA request and can have different policies.
	SessionModeIndependent SessionMode = "independent"
)

type SubscriberGroup struct {
	AccessType   string           `json:"access-type,omitempty" yaml:"access-type,omitempty"` // ipoe, pppoe, lac, lns
	VLANs        []VLANRange      `json:"vlans,omitempty" yaml:"vlans,omitempty"`
	AddressPools []*AddressPool   `json:"address-pools,omitempty" yaml:"address-pools,omitempty"`
	IANAPool     string           `json:"iana-pool,omitempty" yaml:"iana-pool,omitempty"`
	PDPool       string           `json:"pd-pool,omitempty" yaml:"pd-pool,omitempty"`
	SessionMode  SessionMode      `json:"session-mode,omitempty" yaml:"session-mode,omitempty"`
	VLANProtocol string           `json:"vlan-protocol,omitempty" yaml:"vlan-protocol,omitempty"`
	DHCP         *SubscriberDHCP  `json:"dhcp,omitempty" yaml:"dhcp,omitempty"`
	IPv6         *SubscriberIPv6  `json:"ipv6,omitempty" yaml:"ipv6,omitempty"`
	BGP          *SubscriberBGP   `json:"bgp,omitempty" yaml:"bgp,omitempty"`
	VRF                 string           `json:"vrf,omitempty" yaml:"vrf,omitempty"`
	DefaultServiceGroup string           `json:"default-service-group,omitempty" yaml:"default-service-group,omitempty"`
	AAAPolicy    string           `json:"aaa-policy,omitempty" yaml:"aaa-policy,omitempty"`
}

func (sg *SubscriberGroup) GetOuterTPID() uint16 {
	if sg.VLANProtocol == "802.1q" {
		return 0x8100
	}
	return 0x88A8
}

// GetSessionMode returns the configured session mode, defaulting to unified
func (sg *SubscriberGroup) GetSessionMode() SessionMode {
	if sg.SessionMode == "" {
		return SessionModeUnified
	}
	return sg.SessionMode
}

type SubscriberIPv6 struct {
	RA *SubscriberIPv6RA `json:"ra,omitempty" yaml:"ra,omitempty"`
}

type SubscriberIPv6RA struct {
	Managed        *bool  `json:"managed,omitempty" yaml:"managed,omitempty"`
	Other          *bool  `json:"other,omitempty" yaml:"other,omitempty"`
	RouterLifetime uint32 `json:"router_lifetime,omitempty" yaml:"router_lifetime,omitempty"`
	MaxInterval    uint32 `json:"max_interval,omitempty" yaml:"max_interval,omitempty"`
	MinInterval    uint32 `json:"min_interval,omitempty" yaml:"min_interval,omitempty"`
}

type AddressPool struct {
	Name     string   `json:"name,omitempty" yaml:"name,omitempty"`
	Network  string   `json:"network,omitempty" yaml:"network,omitempty"`
	Gateway  string   `json:"gateway,omitempty" yaml:"gateway,omitempty"`
	DNS      []string `json:"dns,omitempty" yaml:"dns,omitempty"`
	Priority int      `json:"priority,omitempty" yaml:"priority,omitempty"`
}

type SubscriberDHCP struct {
	AutoGenerate bool   `json:"auto-generate,omitempty" yaml:"auto-generate,omitempty"`
	LeaseTime    string `json:"lease-time,omitempty" yaml:"lease-time,omitempty"`
}

type SubscriberBGP struct {
	Enabled              bool   `json:"enabled" yaml:"enabled"`
	AdvertisePools       bool   `json:"advertise-pools" yaml:"advertise-pools"`
	RedistributeConnected bool   `json:"redistribute-connected" yaml:"redistribute-connected"`
	VRF                  string `json:"vrf,omitempty" yaml:"vrf,omitempty"`
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
	return sg.AAAPolicy
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
	return vlan.ParseVLANRange(v.SVLAN)
}

func (v *VLANRange) GetCVLAN() (isAny bool, cvlan uint16, err error) {
	return vlan.ParseCVLAN(v.CVLAN)
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
