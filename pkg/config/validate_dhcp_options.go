// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package config

import "fmt"

func (c *Config) validateDHCPOptions() error {
	for profileName, profile := range c.IPv4Profiles {
		if profile == nil {
			continue
		}
		for i := range profile.Pools {
			pool := &profile.Pools[i]
			for j, opt := range pool.Options {
				if err := opt.Validate(); err != nil {
					return fmt.Errorf("ipv4-profiles.%s.pools[%d].dhcp-options[%d]: %w",
						profileName, i, j, err)
				}
			}
		}
	}

	for profileName, profile := range c.IPv6Profiles {
		if profile == nil {
			continue
		}
		for i := range profile.IANAPools {
			pool := &profile.IANAPools[i]
			for j, opt := range pool.Options {
				if err := opt.Validate(); err != nil {
					return fmt.Errorf("ipv6-profiles.%s.iana-pools[%d].dhcpv6-options[%d]: %w",
						profileName, i, j, err)
				}
			}
		}
	}

	return nil
}
