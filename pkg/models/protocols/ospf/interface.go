// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf

type InterfaceMap struct {
	VRF        string               `json:"-"                  metric:"label=vrf,map_key"`
	VRFName    string               `json:"vrfName,omitempty"`
	VRFID      int                  `json:"vrfId,omitempty"`
	Interfaces map[string]Interface `json:"interfaces"         metric:"flatten"`
}

type Interface struct {
	Name               string `json:"-"                    metric:"label=interface,map_key"`
	Area               string `json:"area"                 metric:"label=area"`
	RouterID           string `json:"routerId"`
	State              string `json:"state"                metric:"label"`
	NetworkType        string `json:"networkType,omitempty"`
	OspfIfType         string `json:"ospfIfType,omitempty"`
	IfUp               bool   `json:"ifUp"                 metric:"name=protocols.ospf.interface.up,type=gauge,help=OSPF interface up flag (1 if up)."`
	OspfEnabled        bool   `json:"ospfEnabled"          metric:"name=protocols.ospf.interface.ospf_enabled,type=gauge,help=OSPF protocol enabled on this interface."`
	Cost               int    `json:"cost"                 metric:"name=protocols.ospf.interface.cost,type=gauge,help=OSPF interface cost (output metric)."`
	Priority           int    `json:"priority"             metric:"name=protocols.ospf.interface.priority,type=gauge,help=OSPF interface DR election priority."`
	TimerMsecs         int    `json:"timerMsecs"           metric:"name=protocols.ospf.interface.hello_interval_ms,type=gauge,help=OSPF hello interval in milliseconds."`
	TimerDeadSecs      int    `json:"timerDeadSecs"        metric:"name=protocols.ospf.interface.dead_interval_seconds,type=gauge,help=OSPF dead interval in seconds."`
	TimerRetransmit    int    `json:"timerRetransmitSecs"  metric:"name=protocols.ospf.interface.retransmit_interval_seconds,type=gauge,help=OSPF LSA retransmit interval in seconds."`
	NbrCount           int    `json:"nbrCount"             metric:"name=protocols.ospf.interface.neighbor_count,type=gauge,help=Neighbors discovered on this interface."`
	NbrAdjacentCount   int    `json:"nbrAdjacentCount"     metric:"name=protocols.ospf.interface.neighbor_adjacent_count,type=gauge,help=Fully adjacent neighbors on this interface."`
	LSARetransmissions int    `json:"lsaRetransmissions"   metric:"name=protocols.ospf.interface.lsa_retransmissions_total,type=counter,help=LSA retransmissions on this interface."`
}
