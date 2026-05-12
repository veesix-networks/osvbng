// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf6

type Neighbor struct {
	NeighborID     string `json:"neighborId"     metric:"label"`
	Priority       int    `json:"priority"       metric:"name=protocols.ospf6.neighbor.priority,type=gauge,help=OSPFv3 neighbor priority."`
	DeadTime       string `json:"deadTime"`
	State          string `json:"state"          metric:"label"`
	IfState        string `json:"ifState"`
	Duration       string `json:"duration"`
	InterfaceName  string `json:"interfaceName"  metric:"label"`
	InterfaceState string `json:"interfaceState"`
}
