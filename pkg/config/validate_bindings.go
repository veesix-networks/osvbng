// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"fmt"
	"net"

	"github.com/veesix-networks/osvbng/pkg/netbind"
)

// Commit-time gate so a typo'd vrf or family mismatch fails before any
// reload runs.
func (c *Config) ValidateBindings() error {
	lookup := c.VRFLookup()

	if c.HA.Enabled {
		if err := c.HA.Validate(lookup); err != nil {
			return err
		}
	}

	if !c.API.EndpointBinding.IsZero() {
		if err := c.API.Validate(addrFamily(c.API.Address), lookup); err != nil {
			return fmt.Errorf("api: %w", err)
		}
	}

	for name, p := range c.IPv4Profiles {
		if p == nil || p.DHCP == nil {
			continue
		}
		if err := p.DHCP.Validate(netbind.FamilyV4, lookup); err != nil {
			return fmt.Errorf("ipv4-profiles.%s.dhcp: %w", name, err)
		}
	}
	for name, p := range c.IPv6Profiles {
		if p == nil || p.DHCPv6 == nil {
			continue
		}
		if err := p.DHCPv6.Validate(netbind.FamilyV6, lookup); err != nil {
			return fmt.Errorf("ipv6-profiles.%s.dhcpv6: %w", name, err)
		}
	}
	return nil
}

func addrFamily(addr string) netbind.Family {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
		return netbind.FamilyV6
	}
	return netbind.FamilyV4
}
