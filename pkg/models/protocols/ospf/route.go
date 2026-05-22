// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf

type Route struct {
	RouteType string    `json:"routeType,omitempty"`
	Transit   bool      `json:"transit,omitempty"`
	Cost      int       `json:"cost"`
	Area      string    `json:"area,omitempty"`
	Nexthops  []Nexthop `json:"nexthops,omitempty"`
}

type Nexthop struct {
	IP                 string `json:"ip,omitempty"`
	Via                string `json:"via,omitempty"`
	DirectlyAttachedTo string `json:"directlyAttachedTo,omitempty"`
	AdvertisedRouter   string `json:"advertisedRouter,omitempty"`
}
