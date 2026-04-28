// Copyright 2025 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"fmt"
	"os"

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

func validateVLANTpid(value string) error {
	switch value {
	case "", "dot1q", "dot1ad":
		return nil
	}
	return fmt.Errorf("invalid vlan-tpid %q: use \"dot1q\" or \"dot1ad\" (renamed from vlan-protocol: 802.1q/802.1ad)", value)
}

func (c *Config) Validate() error {
	if _, err := c.GetAccessInterface(); err != nil {
		return fmt.Errorf("access interface validation: %w", err)
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

	if err := c.ValidateBindings(); err != nil {
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
