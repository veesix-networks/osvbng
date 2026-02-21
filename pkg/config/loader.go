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

func (c *Config) Validate() error {
	if _, err := c.GetAccessInterface(); err != nil {
		return fmt.Errorf("access interface validation: %w", err)
	}

	if c.SubscriberGroups != nil {
		for groupName, group := range c.SubscriberGroups.Groups {
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

	return nil
}
