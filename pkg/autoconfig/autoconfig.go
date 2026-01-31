package autoconfig

import (
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/operations"
	"github.com/veesix-networks/osvbng/pkg/paths"
)

type Change struct {
	Path  string
	Value interface{}
}

type Autoconfig struct {
	cfg             *config.Config
	parentInterface string
	puntSocketPath  string
}

func New(cfg *config.Config, parentInterface string) *Autoconfig {
	puntSocketPath := "/run/osvbng/punt.sock"
	if cfg.Dataplane.PuntSocketPath != "" {
		puntSocketPath = cfg.Dataplane.PuntSocketPath
	}

	return &Autoconfig{
		cfg:             cfg,
		parentInterface: parentInterface,
		puntSocketPath:  puntSocketPath,
	}
}

func (a *Autoconfig) DeriveConfig() ([]Change, error) {
	var changes []Change

	if a.cfg.SubscriberGroups == nil {
		return changes, nil
	}

	for _, group := range a.cfg.SubscriberGroups.Groups {
		groupChanges, err := a.deriveGroupConfig(group)
		if err != nil {
			return nil, fmt.Errorf("derive config for group: %w", err)
		}
		changes = append(changes, groupChanges...)
	}

	return changes, nil
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
	encodedSubIfName := paths.EncodeInterfaceName(fmt.Sprintf("%s.%d", a.parentInterface, svlan))
	raConfig := a.getRAConfig(group)

	changes = append(changes, Change{
		Path: fmt.Sprintf("interfaces.%s.subinterfaces.%d", a.parentInterface, svlan),
		Value: &interfaces.SubinterfaceConfig{
			VLAN:    int(svlan),
			Enabled: true,
		},
	})

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

	changes = append(changes, Change{
		Path: fmt.Sprintf("interfaces.%s.subinterfaces.%d.arp", a.parentInterface, svlan),
		Value: &interfaces.ARPConfig{
			Enabled: false,
		},
	})

	changes = append(changes, Change{
		Path:  fmt.Sprintf("interfaces.%s.subinterfaces.%d.unnumbered", a.parentInterface, svlan),
		Value: loopback,
	})

	changes = append(changes, Change{
		Path: fmt.Sprintf("_internal.punt.%s.arp", encodedSubIfName),
		Value: &operations.PuntConfig{
			Enabled:    true,
			SocketPath: a.puntSocketPath,
		},
	})

	changes = append(changes, Change{
		Path: fmt.Sprintf("_internal.punt.%s.dhcpv4", encodedSubIfName),
		Value: &operations.PuntConfig{
			Enabled:    true,
			SocketPath: a.puntSocketPath,
		},
	})

	changes = append(changes, Change{
		Path: fmt.Sprintf("_internal.punt.%s.dhcpv6", encodedSubIfName),
		Value: &operations.PuntConfig{
			Enabled:    true,
			SocketPath: a.puntSocketPath,
		},
	})

	changes = append(changes, Change{
		Path: fmt.Sprintf("_internal.punt.%s.pppoe", encodedSubIfName),
		Value: &operations.PuntConfig{
			Enabled:    true,
			SocketPath: a.puntSocketPath,
		},
	})

	return changes
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
