// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf6

type RouteResponse struct {
	Routes map[string][]Route `json:"routes,omitempty"`
}

type Route struct {
	IsBestRoute     bool      `json:"isBestRoute,omitempty"`
	DestinationType string    `json:"destinationType,omitempty"`
	PathType        string    `json:"pathType,omitempty"`
	Duration        string    `json:"duration,omitempty"`
	NextHops        []Nexthop `json:"nextHops,omitempty"`
}

type Nexthop struct {
	NextHop       string `json:"nextHop,omitempty"`
	InterfaceName string `json:"interfaceName,omitempty"`
}
