// Copyright 2025 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/aaa"
	"github.com/veesix-networks/osvbng/pkg/config/cgnat"
	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/config/ip"
	"github.com/veesix-networks/osvbng/pkg/config/l2tp"
	"github.com/veesix-networks/osvbng/pkg/config/protocols"
	"github.com/veesix-networks/osvbng/pkg/config/qos"
	routing_policy "github.com/veesix-networks/osvbng/pkg/config/routing_policy"
	"github.com/veesix-networks/osvbng/pkg/config/servicegroup"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/config/system"
)

type Config struct {
	// No handlers (skipped by walker)
	Logging          system.LoggingConfig               `json:"logging,omitempty" yaml:"logging,omitempty"`
	Dataplane        system.DataplaneConfig             `json:"dataplane,omitempty" yaml:"dataplane,omitempty"`
	SubscriberGroups *subscriber.SubscriberGroupsConfig `json:"subscriber-groups,omitempty" yaml:"subscriber-groups,omitempty"`
	IPv4Profiles     map[string]*ip.IPv4Profile         `json:"ipv4-profiles,omitempty" yaml:"ipv4-profiles,omitempty"`
	IPv6Profiles     map[string]*ip.IPv6Profile         `json:"ipv6-profiles,omitempty" yaml:"ipv6-profiles,omitempty"`
	DHCP             ip.DHCPConfig                      `json:"dhcp,omitempty" yaml:"dhcp,omitempty"`
	DHCPv6           ip.DHCPv6Config                    `json:"dhcpv6,omitempty" yaml:"dhcpv6,omitempty"`
	Monitoring       system.MonitoringConfig            `json:"monitoring,omitempty" yaml:"monitoring,omitempty"`
	API              system.APIConfig                   `json:"api,omitempty" yaml:"api,omitempty"`
	Watchdog         system.WatchdogConfig              `json:"watchdog,omitempty" yaml:"watchdog,omitempty"`
	CGNAT            *cgnat.Config                      `json:"cgnat,omitempty" yaml:"cgnat,omitempty"`
	HA               HAConfig                           `json:"ha,omitempty" yaml:"ha,omitempty"`
	L2TP             *l2tp.L2TPConfig                   `json:"l2tp,omitempty" yaml:"l2tp,omitempty"`

	// Walked in struct order — dependency order matters
	System          *SystemConfig                          `json:"system,omitempty" yaml:"system,omitempty"`
	RoutingPolicies *routing_policy.RoutingPolicyConfig     `json:"routing-policies,omitempty" yaml:"routing-policies,omitempty"`
	VRFS            map[string]*ip.VRFSConfig               `json:"vrfs,omitempty" yaml:"vrfs,omitempty"`
	QoSPolicies     map[string]*qos.Policy                  `json:"qos-policies,omitempty" yaml:"qos-policies,omitempty"`
	ServiceGroups   map[string]*servicegroup.Config          `json:"service-groups,omitempty" yaml:"service-groups,omitempty"`
	Interfaces      map[string]*interfaces.InterfaceConfig   `json:"interfaces,omitempty" yaml:"interfaces,omitempty"`
	Protocols       protocols.ProtocolConfig                 `json:"protocols,omitempty" yaml:"protocols,omitempty"`
	AAA             aaa.AAAConfig                            `json:"aaa,omitempty" yaml:"aaa,omitempty"`

	// Plugin configs (handled separately)
	Plugins map[string]interface{} `json:"plugins,omitempty" yaml:"plugins,omitempty"`
}

type SystemConfig struct {
	CPPM *system.CPPMConfig `json:"cppm,omitempty" yaml:"cppm,omitempty"`
}

type DiffResult struct {
	Added    []ConfigLine
	Deleted  []ConfigLine
	Modified []ConfigLine
}

type ConfigLine struct {
	Path  string
	Value string
}

// NeedsAccessInterface reports whether this config has any subscriber
// group that requires a directly-attached access interface (PPPoE,
// IPoE, LAC). LNS-only deployments have subscribers arriving via L2TP
// and never bind an access interface.
func (c *Config) NeedsAccessInterface() bool {
	if c.SubscriberGroups == nil {
		return false
	}
	for _, g := range c.SubscriberGroups.Groups {
		if g == nil {
			continue
		}
		if g.HasAccessType("ipoe") || g.HasAccessType("pppoe") || g.HasAccessType("lac") {
			return true
		}
	}
	return false
}

func (c *Config) GetAccessInterface() (string, error) {
	if c.SubscriberGroups == nil {
		return "", fmt.Errorf("no subscriber-group with parent-interface configured")
	}

	seen := map[string]struct{}{}
	for _, g := range c.SubscriberGroups.Groups {
		if g == nil {
			continue
		}
		if !g.HasAccessType("ipoe") && !g.HasAccessType("pppoe") && !g.HasAccessType("lac") {
			continue
		}
		for _, vr := range g.VLANs {
			if vr.ParentInterface != "" {
				seen[vr.ParentInterface] = struct{}{}
			}
		}
	}

	switch len(seen) {
	case 0:
		return "", fmt.Errorf("no subscriber-group vlan-range with parent-interface configured")
	case 1:
		for name := range seen {
			return name, nil
		}
	}

	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	return "", fmt.Errorf("multiple parent-interfaces configured across subscriber groups (only 1 allowed): %v", names)
}

func (c *Config) GetCoreInterface() string {
	for name, iface := range c.Interfaces {
		if iface.BNGMode == "core" {
			return name
		}
	}
	return ""
}

