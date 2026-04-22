// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package subscriber

const (
	DefaultSubscriberPathMTU uint16 = 1500

	tcpV4HeaderOverhead uint16 = 40
	tcpV6HeaderOverhead uint16 = 60
)

type MSSClampConfig struct {
	Enabled           *bool   `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	SubscriberPathMTU *uint16 `json:"subscriber-path-mtu,omitempty" yaml:"subscriber-path-mtu,omitempty"`
	IPv4MSS           *uint16 `json:"ipv4-mss,omitempty" yaml:"ipv4-mss,omitempty"`
	IPv6MSS           *uint16 `json:"ipv6-mss,omitempty" yaml:"ipv6-mss,omitempty"`
}

func (c *MSSClampConfig) IsEnabled() bool {
	if c == nil || c.Enabled == nil {
		return true
	}
	return *c.Enabled
}

func (c *MSSClampConfig) GetSubscriberPathMTU() uint16 {
	if c == nil || c.SubscriberPathMTU == nil {
		return DefaultSubscriberPathMTU
	}
	return *c.SubscriberPathMTU
}

func (c *MSSClampConfig) IPv4MSSAuto() uint16 {
	if c != nil && c.IPv4MSS != nil {
		return *c.IPv4MSS
	}
	mtu := c.GetSubscriberPathMTU()
	if mtu < tcpV4HeaderOverhead {
		return 0
	}
	return mtu - tcpV4HeaderOverhead
}

func (c *MSSClampConfig) IPv6MSSAuto() uint16 {
	if c != nil && c.IPv6MSS != nil {
		return *c.IPv6MSS
	}
	mtu := c.GetSubscriberPathMTU()
	if mtu < tcpV6HeaderOverhead {
		return 0
	}
	return mtu - tcpV6HeaderOverhead
}

func (c *MSSClampConfig) IPv4MSSOrAuto(mtu uint16) uint16 {
	if c != nil && c.IPv4MSS != nil {
		return *c.IPv4MSS
	}
	if mtu < tcpV4HeaderOverhead {
		return 0
	}
	return mtu - tcpV4HeaderOverhead
}

func (c *MSSClampConfig) IPv6MSSOrAuto(mtu uint16) uint16 {
	if c != nil && c.IPv6MSS != nil {
		return *c.IPv6MSS
	}
	if mtu < tcpV6HeaderOverhead {
		return 0
	}
	return mtu - tcpV6HeaderOverhead
}
