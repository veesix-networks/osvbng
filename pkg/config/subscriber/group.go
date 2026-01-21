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

type SubscriberGroup struct {
	VLANs        []VLANRange     `json:"vlans,omitempty" yaml:"vlans,omitempty"`
	AddressPools []*AddressPool  `json:"address-pools,omitempty" yaml:"address-pools,omitempty"`
	DHCP         *SubscriberDHCP `json:"dhcp,omitempty" yaml:"dhcp,omitempty"`
	BGP          *SubscriberBGP  `json:"bgp,omitempty" yaml:"bgp,omitempty"`
	VRF          string          `json:"vrf,omitempty" yaml:"vrf,omitempty"`
	AAAPolicy    string          `json:"aaa-policy,omitempty" yaml:"aaa-policy,omitempty"`
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
