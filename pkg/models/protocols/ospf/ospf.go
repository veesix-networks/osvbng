// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf

type Neighbor struct {
	RouterID                           string `json:"-" metric:"label=neighbor_router_id,map_key"`
	NbrState                           string `json:"nbrState"                            metric:"label"`
	NbrPriority                        int    `json:"nbrPriority"                         metric:"name=protocols.ospf.neighbor.priority,type=gauge,help=OSPF neighbor priority."`
	Converged                          string `json:"converged"`
	Role                               string `json:"role"`
	UpTimeInMsec                       int    `json:"upTimeInMsec"                        metric:"name=protocols.ospf.neighbor.uptime_ms,type=gauge,help=OSPF neighbor uptime in milliseconds."`
	RouterDeadIntervalTimerDueMsec     int    `json:"routerDeadIntervalTimerDueMsec"      metric:"name=protocols.ospf.neighbor.dead_timer_due_ms,type=gauge,help=Milliseconds until the OSPF dead timer fires."`
	UpTime                             string `json:"upTime"`
	DeadTime                           string `json:"deadTime"`
	IfaceAddress                       string `json:"ifaceAddress"`
	IfaceName                          string `json:"ifaceName"                           metric:"label"`
	LinkStateRetransmissionListCounter int    `json:"linkStateRetransmissionListCounter"  metric:"name=protocols.ospf.neighbor.lsr_retransmit_queue,type=gauge,help=OSPF link-state retransmission queue depth."`
	LinkStateRequestListCounter        int    `json:"linkStateRequestListCounter"         metric:"name=protocols.ospf.neighbor.lsr_request_queue,type=gauge,help=OSPF link-state request queue depth."`
	DatabaseSummaryListCounter         int    `json:"databaseSummaryListCounter"          metric:"name=protocols.ospf.neighbor.db_summary_queue,type=gauge,help=OSPF database-summary queue depth."`
}
