// Copyright 2025 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"fmt"
	"net"
	"os"

	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/config/system"
	"gopkg.in/yaml.v3"
)

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.applyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &cfg, nil
}

func Save(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	return nil
}

func (c *Config) applyDefaults() {
	defaults := system.DefaultWatchdogConfig()

	if c.Watchdog.CheckInterval == 0 {
		c.Watchdog.CheckInterval = defaults.CheckInterval
	}
	if c.Watchdog.Targets == nil {
		c.Watchdog.Targets = defaults.Targets
	} else {
		for name, defaultTarget := range defaults.Targets {
			if _, exists := c.Watchdog.Targets[name]; !exists {
				c.Watchdog.Targets[name] = defaultTarget
			}
		}
	}
}

func (c *Config) resolveVxlanSrc(ifName string, iface *interfaces.InterfaceConfig) error {
	if iface.Vxlan.SrcInterface == "" {
		return nil
	}

	srcIface, ok := c.Interfaces[iface.Vxlan.SrcInterface]
	if !ok || srcIface == nil {
		return fmt.Errorf("src-interface references unknown interface %q", iface.Vxlan.SrcInterface)
	}
	if srcIface.Address == nil || len(srcIface.Address.IPv4) == 0 {
		return fmt.Errorf("src-interface %q has no ipv4 address", iface.Vxlan.SrcInterface)
	}

	ip, _, err := net.ParseCIDR(srcIface.Address.IPv4[0])
	if err != nil {
		return fmt.Errorf("src-interface %q address %q: %w", iface.Vxlan.SrcInterface, srcIface.Address.IPv4[0], err)
	}

	iface.Vxlan.Src = ip.String()
	return nil
}

func (c *Config) validatePseudowires() error {
	transportUsers := make(map[string]string)

	for ifName, iface := range c.Interfaces {
		if iface == nil || iface.Pseudowire == nil {
			continue
		}

		if err := iface.Pseudowire.Validate(); err != nil {
			return fmt.Errorf("interfaces.%s.pseudowire: %w", ifName, err)
		}

		transport := iface.Pseudowire.Transport
		tIface, ok := c.Interfaces[transport]
		if !ok || tIface == nil {
			return fmt.Errorf("interfaces.%s.pseudowire: transport references unknown interface %q", ifName, transport)
		}
		if tIface.Vxlan == nil {
			return fmt.Errorf("interfaces.%s.pseudowire: transport %q is not a tunnel interface", ifName, transport)
		}
		if prev, used := transportUsers[transport]; used {
			return fmt.Errorf("interfaces.%s.pseudowire: transport %q already used by pseudowire %q", ifName, transport, prev)
		}
		transportUsers[transport] = ifName

		if c.L2GW != nil && c.L2GW.HandoffGroups != nil {
			for hgName, hg := range c.L2GW.HandoffGroups {
				if hg != nil && hg.Interface == transport {
					return fmt.Errorf("interfaces.%s.pseudowire: transport %q is an l2gw handoff interface (handoff-group %s)", ifName, transport, hgName)
				}
			}
		}
		if c.SubscriberGroups != nil {
			for groupName, group := range c.SubscriberGroups.Groups {
				for i, vr := range group.VLANs {
					if vr.ParentInterface == transport {
						return fmt.Errorf("interfaces.%s.pseudowire: transport %q is a subscriber-group parent interface (subscriber_groups.%s.vlans[%d])", ifName, transport, groupName, i)
					}
				}
			}
		}

		tIface.Vxlan.PWTransport = true
	}

	return nil
}

func validateVLANTpid(value string) error {
	switch value {
	case "", "dot1q", "dot1ad":
		return nil
	}
	return fmt.Errorf("invalid vlan-tpid %q: use \"dot1q\" or \"dot1ad\" (renamed from vlan-protocol: 802.1q/802.1ad)", value)
}

func (c *Config) Validate() error {
	if err := ValidateSubscriberAccessTypes(c); err != nil {
		return err
	}

	if err := c.CGNAT.Validate(); err != nil {
		return err
	}

	if c.NeedsAccessInterface() {
		if _, err := c.GetAccessInterface(); err != nil {
			return fmt.Errorf("access interface validation: %w", err)
		}
	}

	for ifName, iface := range c.Interfaces {
		if iface == nil {
			continue
		}
		for subID, sub := range iface.Subinterfaces {
			if sub == nil {
				continue
			}
			if err := validateVLANTpid(sub.VLANTpid); err != nil {
				return fmt.Errorf("interfaces.%s.subinterfaces.%s: %w", ifName, subID, err)
			}
		}
		if iface.Vxlan != nil && iface.Pseudowire != nil {
			return fmt.Errorf("interfaces.%s: vxlan and pseudowire are mutually exclusive", ifName)
		}
		if iface.Vxlan != nil {
			if err := iface.Vxlan.Validate(); err != nil {
				return fmt.Errorf("interfaces.%s.vxlan: %w", ifName, err)
			}
			if err := c.resolveVxlanSrc(ifName, iface); err != nil {
				return fmt.Errorf("interfaces.%s.vxlan: %w", ifName, err)
			}
		}
	}

	evpnVNIs := make(map[uint32]string)
	for ifName, iface := range c.Interfaces {
		if iface == nil || iface.Vxlan == nil || !iface.Vxlan.EVPNSignaled() {
			continue
		}
		if prev, ok := evpnVNIs[iface.Vxlan.VNI]; ok {
			return fmt.Errorf("interfaces.%s.vxlan: vni %d already used by evpn-signaled interface %q", ifName, iface.Vxlan.VNI, prev)
		}
		evpnVNIs[iface.Vxlan.VNI] = ifName
	}

	if c.SubscriberGroups != nil {
		for groupName, group := range c.SubscriberGroups.Groups {
			if err := validateVLANTpid(group.VLANTpid); err != nil {
				return fmt.Errorf("subscriber_groups.%s: %w", groupName, err)
			}
			for i, vlanRange := range group.VLANs {
				if _, err := vlanRange.GetSVLANs(); err != nil {
					return fmt.Errorf("subscriber_groups.%s.vlans[%d].svlan: %w", groupName, i, err)
				}

				if _, _, err := vlanRange.GetCVLAN(); err != nil {
					return fmt.Errorf("subscriber_groups.%s.vlans[%d].cvlan: %w", groupName, i, err)
				}

				if vlanRange.DHCP != "" {
					if c.DHCP.GetServer(vlanRange.DHCP) == nil {
						return fmt.Errorf("subscriber_groups.%s.vlans[%d].dhcp references unknown server '%s'", groupName, i, vlanRange.DHCP)
					}
				}

				if vlanRange.AAA != nil {
					if vlanRange.AAA.Policy != "" {
						if c.AAA.GetPolicy(vlanRange.AAA.Policy) == nil {
							return fmt.Errorf("subscriber_groups.%s.vlans[%d].aaa.policy references unknown policy '%s'", groupName, i, vlanRange.AAA.Policy)
						}
					}
				}
			}

			if group.AAAPolicy != "" {
				if c.AAA.GetPolicy(group.AAAPolicy) == nil {
					return fmt.Errorf("subscriber_groups.%s.aaa_policy references unknown policy '%s'", groupName, group.AAAPolicy)
				}
			}
		}
	}

	if err := c.validatePseudowires(); err != nil {
		return err
	}

	if err := c.ValidateBindings(); err != nil {
		return err
	}

	// At some point, we should probably remove this specific
	// validation steps and utilize the actual Validate() methods on
	// the respective config handlers, or build a better SDK for validating
	// not only in realtime but on startup instead of building a bespoke validation
	// method every time we need to validate on the config startup etc..
	if err := c.validateDHCPOptions(); err != nil {
		return err
	}

	if err := c.validateOSPFVRFInterfaces(); err != nil {
		return err
	}

	if err := c.L2GW.Validate(); err != nil {
		return err
	}

	if c.HA.Enabled && c.SubscriberGroups != nil {
		for srgName, srg := range c.HA.SRGs {
			for _, sg := range srg.SubscriberGroups {
				if _, ok := c.SubscriberGroups.Groups[sg]; !ok {
					return fmt.Errorf("ha.srgs.%s.subscriber_groups: references unknown subscriber group %q", srgName, sg)
				}
			}
		}
	}

	return nil
}
