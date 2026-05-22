package autoconfig

import (
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/operations"
	pkgpaths "github.com/veesix-networks/osvbng/pkg/paths"
)

type Change struct {
	Path  string
	Value interface{}
}

type Autoconfig struct {
	cfg             *config.Config
	parentInterface string
}

func New(cfg *config.Config, parentInterface string) *Autoconfig {
	return &Autoconfig{
		cfg:             cfg,
		parentInterface: parentInterface,
	}
}

func (a *Autoconfig) DeriveConfig() ([]Change, error) {
	if a.cfg.SubscriberGroups == nil {
		return nil, nil
	}

	var ordered []Change
	seen := map[string]int{}

	for _, group := range a.cfg.SubscriberGroups.Groups {
		groupChanges, err := a.deriveGroupConfig(group)
		if err != nil {
			return nil, fmt.Errorf("derive config for group: %w", err)
		}
		for _, ch := range groupChanges {
			if idx, ok := seen[ch.Path]; ok {
				ordered[idx] = ch
				continue
			}
			seen[ch.Path] = len(ordered)
			ordered = append(ordered, ch)
		}
	}

	return ordered, nil
}

func (a *Autoconfig) deriveGroupConfig(group *subscriber.SubscriberGroup) ([]Change, error) {
	var changes []Change

	for _, vlanRange := range group.VLANs {
		svlans, err := vlanRange.GetSVLANs()
		if err != nil {
			return nil, fmt.Errorf("parse svlan range: %w", err)
		}

		for _, svlan := range svlans {
			svlanChanges := a.deriveSVLANConfig(group, vlanRange, svlan)
			changes = append(changes, svlanChanges...)
		}
	}

	return changes, nil
}

func (a *Autoconfig) deriveSVLANConfig(group *subscriber.SubscriberGroup, vlanRange subscriber.VLANRange, svlan uint16) []Change {
	var changes []Change

	loopback := vlanRange.Interface
	raConfig := a.getRAConfig(group)
	resolvedVRF := resolveRangeVRF(group, vlanRange)
	vrfHasIPv6 := a.vrfHasIPv6Unicast(resolvedVRF)

	changes = append(changes, Change{
		Path: fmt.Sprintf("interfaces.%s.subinterfaces.%d", a.parentInterface, svlan),
		Value: &interfaces.SubinterfaceConfig{
			ID:               int(svlan),
			VLAN:             int(svlan),
			VLANTpid:         group.VLANTpid,
			Enabled:          true,
			Unnumbered:       loopback,
			SubscriberAccess: true,
			MSSClamp:         a.deriveMSSClamp(group, vlanRange),
		},
	})

	if vrfHasIPv6 {
		changes = append(changes, Change{
			Path: fmt.Sprintf("interfaces.%s.ipv6", loopback),
			Value: &interfaces.IPv6Config{
				Enabled: true,
			},
		})

		changes = append(changes, Change{
			Path: fmt.Sprintf("interfaces.%s.subinterfaces.%d.ipv6", a.parentInterface, svlan),
			Value: &interfaces.IPv6Config{
				Enabled:   true,
				Multicast: true,
				RA: &interfaces.RAConfig{
					Managed:        raConfig.Managed,
					Other:          raConfig.Other,
					RouterLifetime: raConfig.RouterLifetime,
					MaxInterval:    raConfig.MaxInterval,
					MinInterval:    raConfig.MinInterval,
				},
			},
		})
	}

	changes = append(changes, Change{
		Path: fmt.Sprintf("interfaces.%s.subinterfaces.%d.arp", a.parentInterface, svlan),
		Value: &interfaces.ARPConfig{
			Enabled: false,
		},
	})

	if resolvedVRF != "" {
		changes = append(changes, Change{
			Path:  fmt.Sprintf("interfaces.%s.subinterfaces.%d.vrf", a.parentInterface, svlan),
			Value: resolvedVRF,
		})
	}

	changes = append(changes, Change{
		Path:  fmt.Sprintf("interfaces.%s.subinterfaces.%d.unnumbered", a.parentInterface, svlan),
		Value: loopback,
	})

	subIfEncoded := pkgpaths.EncodeInterfaceName(fmt.Sprintf("%s.%d", a.parentInterface, svlan))
	parentEncoded := pkgpaths.EncodeInterfaceName(a.parentInterface)
	if vlanRange.HasAccessType(subscriber.AccessTypeIPoE) {
		ipoePunts := []string{"dhcpv4", "arp"}
		if vrfHasIPv6 {
			ipoePunts = append(ipoePunts, "dhcpv6", "ipv6nd")
		}
		for _, proto := range ipoePunts {
			changes = append(changes, Change{
				Path:  fmt.Sprintf("_internal.punt.%s.%s", subIfEncoded, proto),
				Value: &operations.PuntConfig{Enabled: true},
			})
		}
		changes = append(changes, Change{
			Path:  fmt.Sprintf("_internal.access.%s.ipoe-input", subIfEncoded),
			Value: &operations.AccessConfig{Enabled: true},
		})
	}
	if vlanRange.HasAccessType(subscriber.AccessTypePPPoE) || vlanRange.HasAccessType(subscriber.AccessTypeLAC) {
		changes = append(changes, Change{
			Path:  fmt.Sprintf("_internal.punt.%s.pppoe", subIfEncoded),
			Value: &operations.PuntConfig{Enabled: true},
		})
		changes = append(changes, Change{
			Path:  fmt.Sprintf("_internal.access.%s.promiscuous", parentEncoded),
			Value: &operations.AccessConfig{Enabled: true},
		})
	}
	if vlanRange.HasAccessType(subscriber.AccessTypeLNS) {
		changes = append(changes, Change{
			Path:  fmt.Sprintf("_internal.punt.%s.l2tp", subIfEncoded),
			Value: &operations.PuntConfig{Enabled: true},
		})
	}

	return changes
}

func resolveRangeVRF(group *subscriber.SubscriberGroup, r subscriber.VLANRange) string {
	if r.VRF == "default" {
		return ""
	}
	if r.VRF != "" {
		return r.VRF
	}
	if group.VRF == "default" {
		return ""
	}
	return group.VRF
}

type raConfig struct {
	Managed        bool
	Other          bool
	RouterLifetime uint32
	MaxInterval    uint32
	MinInterval    uint32
}

func (a *Autoconfig) getRAConfig(group *subscriber.SubscriberGroup) raConfig {
	globalRA := a.cfg.DHCPv6.RA

	cfg := raConfig{
		Managed:        globalRA.GetManaged(),
		Other:          globalRA.GetOther(),
		RouterLifetime: globalRA.GetRouterLifetime(),
		MaxInterval:    globalRA.GetMaxInterval(),
		MinInterval:    globalRA.GetMinInterval(),
	}

	if group.IPv6 != nil && group.IPv6.RA != nil {
		groupRA := group.IPv6.RA
		if groupRA.Managed != nil {
			cfg.Managed = *groupRA.Managed
		}
		if groupRA.Other != nil {
			cfg.Other = *groupRA.Other
		}
		if groupRA.RouterLifetime != 0 {
			cfg.RouterLifetime = groupRA.RouterLifetime
		}
		if groupRA.MaxInterval != 0 {
			cfg.MaxInterval = groupRA.MaxInterval
		}
		if groupRA.MinInterval != 0 {
			cfg.MinInterval = groupRA.MinInterval
		}
	}

	return cfg
}

// deriveMSSClamp computes the MSS clamp policy for an IPoE access sub-interface.
// PPPoE access is handled per-session in internal/pppoe (the negotiated PPP MTU
// is the source of truth there, not the subscriber-group's path MTU default).
//
// MSS is derived from the subscriber-group's subscriber-path-mtu, NOT from the
// BNG access interface's local MTU. The two are independent: the BNG may run
// jumbo frames on the access link to support MPLS, SR-MPLS, or pseudowire
// termination while the actual subscriber CPE on the far side is still 1500-
// or-less. Operators on non-standard subscriber paths must declare
// subscriber-path-mtu explicitly per group.
func (a *Autoconfig) deriveMSSClamp(group *subscriber.SubscriberGroup, vlanRange subscriber.VLANRange) *interfaces.MSSClampSpec {
	if len(vlanRange.AccessTypes) == 1 && vlanRange.HasAccessType(subscriber.AccessTypePPPoE) {
		return nil
	}
	if !group.MSSClamp.IsEnabled() {
		return &interfaces.MSSClampSpec{Enabled: false}
	}
	return &interfaces.MSSClampSpec{
		Enabled: true,
		IPv4MSS: group.MSSClamp.IPv4MSSAuto(),
		IPv6MSS: group.MSSClamp.IPv6MSSAuto(),
	}
}

func (a *Autoconfig) vrfHasIPv6Unicast(vrfName string) bool {
	if vrfName == "" {
		return true
	}
	if a.cfg == nil || a.cfg.VRFS == nil {
		return true
	}
	v, ok := a.cfg.VRFS[vrfName]
	if !ok {
		return true
	}
	return v.AddressFamilies.IPv6Unicast != nil
}
