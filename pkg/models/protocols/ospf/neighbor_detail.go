// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf

type NeighborDetailMap struct {
	VRF       string                      `json:"-"                  metric:"label=vrf,map_key"`
	VRFName   string                      `json:"vrfName,omitempty"`
	VRFID     int                         `json:"vrfId,omitempty"`
	Neighbors map[string][]NeighborDetail `json:"neighbors"          metric:"flatten"`
}

type NeighborDetail struct {
	RouterID                             string `json:"-"                                          metric:"label=neighbor_router_id,map_key"`
	AreaID                               string `json:"areaId"                                     metric:"label=area"`
	IfaceName                            string `json:"ifaceName"                                  metric:"label=interface"`
	IfaceAddress                         string `json:"ifaceAddress,omitempty"`
	LocalIfaceAddress                    string `json:"localIfaceAddress,omitempty"`
	NbrState                             string `json:"nbrState"                                   metric:"label"`
	NbrPriority                          int    `json:"nbrPriority"`
	Role                                 string `json:"role,omitempty"`
	OptionsList                          string `json:"optionsList,omitempty"`
	GrHelperStatus                       string `json:"grHelperStatus,omitempty"                   metric:"label=gr_helper_status"`
	StateChangeCounter                   int    `json:"stateChangeCounter"                         metric:"name=protocols.ospf.neighbor_detail.state_change_total,type=counter,help=Total OSPF neighbor state changes."`
	LsaRetransmissions                   int    `json:"lsaRetransmissions"                         metric:"name=protocols.ospf.neighbor_detail.lsa_retransmissions_total,type=counter,help=Total LSA retransmissions for this neighbor."`
	LastPrgrsvChangeMsec                 int    `json:"lastPrgrsvChangeMsec"                       metric:"name=protocols.ospf.neighbor_detail.last_progressive_change_ms,type=gauge,help=Milliseconds since the last progressive state change."`
	OptionsCounter                       int    `json:"optionsCounter"                             metric:"name=protocols.ospf.neighbor_detail.options_counter,type=gauge,help=OSPF options field counter."`
	RouterDeadIntervalTimerDueMsec       int    `json:"routerDeadIntervalTimerDueMsec"             metric:"name=protocols.ospf.neighbor_detail.dead_timer_due_ms,type=gauge,help=Milliseconds until the dead timer fires."`
	DatabaseSummaryListCounter           int    `json:"databaseSummaryListCounter"                 metric:"name=protocols.ospf.neighbor_detail.db_summary_queue,type=gauge,help=Database-summary queue depth."`
	LinkStateRequestListCounter          int    `json:"linkStateRequestListCounter"                metric:"name=protocols.ospf.neighbor_detail.lsr_request_queue,type=gauge,help=Link-state request queue depth."`
	LinkStateRetransmissionListCounter   int    `json:"linkStateRetransmissionListCounter"         metric:"name=protocols.ospf.neighbor_detail.lsr_retransmit_queue,type=gauge,help=Link-state retransmission queue depth."`
	ThreadInactivityTimer                string `json:"threadInactivityTimer,omitempty"`
	ThreadLinkStateRequestRetransmission string `json:"threadLinkStateRequestRetransmission,omitempty"`
}
