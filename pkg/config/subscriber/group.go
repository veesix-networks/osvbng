package subscriber

import (
	"github.com/veesix-networks/osvbng/pkg/config/vlan"
)

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
