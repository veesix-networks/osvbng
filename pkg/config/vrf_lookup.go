// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package config

import "github.com/veesix-networks/osvbng/pkg/netbind"

func (c *Config) VRFLookup() netbind.VRFLookup {
	return func(name string) (netbind.VRFInfo, bool) {
		if c == nil {
			return netbind.VRFInfo{}, false
		}
		v, ok := c.VRFS[name]
		if !ok {
			return netbind.VRFInfo{}, false
		}
		return netbind.VRFInfo{
			IPv4: v.AddressFamilies.IPv4Unicast != nil,
			IPv6: v.AddressFamilies.IPv6Unicast != nil,
		}, true
	}
}
