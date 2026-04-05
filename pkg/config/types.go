// Copyright 2025 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/aaa"
	"github.com/veesix-networks/osvbng/pkg/config/cgnat"
	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/config/ip"
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

func (c *Config) GetAccessInterface() (string, error) {
	var accessInterfaces []string

	for name, iface := range c.Interfaces {
		if iface.BNGMode == "access" {
			accessInterfaces = append(accessInterfaces, name)
		}
	}

	if len(accessInterfaces) == 0 {
		return "", fmt.Errorf("no interface configured with bng_mode: access")
	}

	if len(accessInterfaces) > 1 {
		return "", fmt.Errorf("multiple interfaces configured with bng_mode: access (only 1 allowed): %v", accessInterfaces)
	}

	return accessInterfaces[0], nil
}

func (c *Config) GetCoreInterface() string {
	for name, iface := range c.Interfaces {
		if iface.BNGMode == "core" {
			return name
		}
	}
	return ""
}
