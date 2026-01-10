package config

import (
	"fmt"
	"os"

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

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &cfg, nil
}

func (c *Config) Validate() error {
	if c.Dataplane.AccessInterface == "" {
		return fmt.Errorf("dataplane.access_interface is required")
	}

	for i, vlanRange := range c.SubscriberGroup.VLANs {
		if _, err := vlanRange.GetSVLANs(); err != nil {
			return fmt.Errorf("subscriber_group.vlans[%d].svlan: %w", i, err)
		}

		if _, _, err := vlanRange.GetCVLAN(); err != nil {
			return fmt.Errorf("subscriber_group.vlans[%d].cvlan: %w", i, err)
		}

		if vlanRange.DHCP != "" {
			if c.DHCP.GetServer(vlanRange.DHCP) == nil {
				return fmt.Errorf("subscriber_group.vlans[%d].dhcp references unknown server '%s'", i, vlanRange.DHCP)
			}
		}

		if vlanRange.AAA != nil {
			if vlanRange.AAA.Policy != "" {
				if c.AAA.GetPolicy(vlanRange.AAA.Policy) == nil {
					return fmt.Errorf("subscriber_group.vlans[%d].aaa.policy references unknown policy '%s'", i, vlanRange.AAA.Policy)
				}
			}

			if vlanRange.AAA.RADIUS != "" {
				if c.AAA.GetRADIUSGroup(vlanRange.AAA.RADIUS) == nil {
					return fmt.Errorf("subscriber_group.vlans[%d].aaa.radius references unknown radius group '%s'", i, vlanRange.AAA.RADIUS)
				}
			}
		}
	}

	return nil
}
