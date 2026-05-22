// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf

type Instance struct {
	VRF                    string                  `json:"-"                       metric:"label=vrf,map_key"`
	VRFName                string                  `json:"vrfName,omitempty"`
	VRFID                  int                     `json:"vrfId,omitempty"`
	RouterID               string                  `json:"routerId"`
	AttachedAreaCounter    int                     `json:"attachedAreaCounter"    metric:"name=protocols.ospf.attached_area_count,type=gauge,help=Number of OSPF areas attached to this router."`
	SpfLastDurationMsecs   int                     `json:"spfLastDurationMsecs"   metric:"name=protocols.ospf.spf_last_duration_ms,type=gauge,help=Duration of the last SPF run in milliseconds."`
	SpfLastExecutedMsecs   int                     `json:"spfLastExecutedMsecs"   metric:"name=protocols.ospf.spf_last_executed_ms,type=gauge,help=Milliseconds since the last SPF run."`
	LsaExternalCounter     int                     `json:"lsaExternalCounter"     metric:"name=protocols.ospf.lsa_external_count,type=gauge,help=Number of Type-5 external LSAs in the LSDB."`
	LsaAsopaqueCounter     int                     `json:"lsaAsopaqueCounter"     metric:"name=protocols.ospf.lsa_as_opaque_count,type=gauge,help=Number of AS-scoped opaque LSAs in the LSDB."`
	MaximumPaths           int                     `json:"maximumPaths"           metric:"name=protocols.ospf.maximum_paths,type=gauge,help=OSPF ECMP maximum-paths setting."`
	Preference             int                     `json:"preference"             metric:"name=protocols.ospf.preference,type=gauge,help=OSPF administrative distance."`
	RefreshTimerMsecs      int                     `json:"refreshTimerMsecs"      metric:"name=protocols.ospf.refresh_timer_ms,type=gauge,help=OSPF LSA refresh timer in milliseconds."`
	Areas                  map[string]InstanceArea `json:"areas"                  metric:"flatten"`
}

type InstanceArea struct {
	Area                   string `json:"-"                       metric:"label=area,map_key"`
	Backbone               bool   `json:"backbone,omitempty"`
	AreaIfTotalCounter     int    `json:"areaIfTotalCounter"     metric:"name=protocols.ospf.area.interface_total,type=gauge,help=Total OSPF interfaces in this area."`
	AreaIfActiveCounter    int    `json:"areaIfActiveCounter"    metric:"name=protocols.ospf.area.interface_active,type=gauge,help=Active OSPF interfaces in this area."`
	NbrFullAdjacentCounter int    `json:"nbrFullAdjacentCounter" metric:"name=protocols.ospf.area.neighbor_full,type=gauge,help=Fully-adjacent OSPF neighbors in this area."`
	SpfExecutedCounter     int    `json:"spfExecutedCounter"     metric:"name=protocols.ospf.area.spf_executed_total,type=counter,help=Total SPF executions for this area."`
	LsaNumber              int    `json:"lsaNumber"              metric:"name=protocols.ospf.area.lsa_count,type=gauge,help=Total LSAs in this area's LSDB."`
	LsaRouterNumber        int    `json:"lsaRouterNumber"        metric:"name=protocols.ospf.area.lsa_router_count,type=gauge,help=Router LSAs in this area."`
	LsaNetworkNumber       int    `json:"lsaNetworkNumber"       metric:"name=protocols.ospf.area.lsa_network_count,type=gauge,help=Network LSAs in this area."`
	LsaSummaryNumber       int    `json:"lsaSummaryNumber"       metric:"name=protocols.ospf.area.lsa_summary_count,type=gauge,help=Summary LSAs in this area."`
	LsaAsbrNumber          int    `json:"lsaAsbrNumber"          metric:"name=protocols.ospf.area.lsa_asbr_count,type=gauge,help=ASBR-summary LSAs in this area."`
	LsaNssaNumber          int    `json:"lsaNssaNumber"          metric:"name=protocols.ospf.area.lsa_nssa_count,type=gauge,help=NSSA-external LSAs in this area."`
	LsaOpaqueLinkNumber    int    `json:"lsaOpaqueLinkNumber"    metric:"name=protocols.ospf.area.lsa_opaque_link_count,type=gauge,help=Link-scoped opaque LSAs in this area."`
	LsaOpaqueAreaNumber    int    `json:"lsaOpaqueAreaNumber"    metric:"name=protocols.ospf.area.lsa_opaque_area_count,type=gauge,help=Area-scoped opaque LSAs in this area."`
}
