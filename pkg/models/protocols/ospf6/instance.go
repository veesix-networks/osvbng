// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf6

type Instance struct {
	RouterID               string                  `json:"routerId"`
	Running                string                  `json:"running,omitempty"`
	MaximumPaths           int                     `json:"maximumPaths"           metric:"name=protocols.ospf6.maximum_paths,type=gauge,help=OSPFv3 ECMP maximum-paths setting."`
	Preference             int                     `json:"preference"             metric:"name=protocols.ospf6.preference,type=gauge,help=OSPFv3 administrative distance."`
	HoldTimeMinMsecs       int                     `json:"holdTimeMinMsecs"       metric:"name=protocols.ospf6.holdtime_min_ms,type=gauge,help=OSPFv3 SPF holdtime minimum in milliseconds."`
	HoldTimeMaxMsecs       int                     `json:"holdTimeMaxMsecs"       metric:"name=protocols.ospf6.holdtime_max_ms,type=gauge,help=OSPFv3 SPF holdtime maximum in milliseconds."`
	HoldTimeMultiplier     int                     `json:"holdTimeMultiplier"     metric:"name=protocols.ospf6.holdtime_multiplier,type=gauge,help=OSPFv3 SPF holdtime multiplier."`
	LsaMinimumArrivalMsecs int                     `json:"lsaMinimumArrivalMsecs" metric:"name=protocols.ospf6.lsa_min_arrival_ms,type=gauge,help=OSPFv3 minimum LSA arrival interval in milliseconds."`
	SpfScheduleDelayMsecs  int                     `json:"spfScheduleDelayMsecs"  metric:"name=protocols.ospf6.spf_schedule_delay_ms,type=gauge,help=OSPFv3 SPF schedule delay in milliseconds."`
	SpfLastDurationMsecs   int                     `json:"spfLastDurationMsecs"   metric:"name=protocols.ospf6.spf_last_duration_ms,type=gauge,help=Duration of the last OSPFv3 SPF run in milliseconds."`
	SpfLastDurationSecs    int                     `json:"spfLastDurationSecs"`
	SpfLastExecutedMsecs   string                  `json:"spfLastExecutedMsecs,omitempty"`
	SpfLastExecutedReason  string                  `json:"spfLastExecutedReason,omitempty"`
	NumberOfAsScopedLsa    int                     `json:"numberOfAsScopedLsa"    metric:"name=protocols.ospf6.as_scoped_lsa_count,type=gauge,help=OSPFv3 AS-scoped LSA count."`
	NumberOfAreaInRouter   int                     `json:"numberOfAreaInRouter"   metric:"name=protocols.ospf6.attached_area_count,type=gauge,help=OSPFv3 areas attached to this router."`
	AdjacencyChanges       string                  `json:"adjacencyChanges,omitempty"`
	Areas                  map[string]InstanceArea `json:"areas"                  metric:"flatten"`
}

type InstanceArea struct {
	Area                  string `json:"-"                     metric:"label=area,map_key"`
	AreaIsStub            bool   `json:"areaIsStub,omitempty"`
	AreaIsNSSA            bool   `json:"areaIsNSSA,omitempty"`
	NumberOfAreaScopedLsa int    `json:"numberOfAreaScopedLsa" metric:"name=protocols.ospf6.area.scoped_lsa_count,type=gauge,help=OSPFv3 area-scoped LSA count."`
	SpfLastExecutedSecs   int    `json:"spfLastExecutedSecs"   metric:"name=protocols.ospf6.area.spf_last_executed_seconds,type=gauge,help=Seconds since the last OSPFv3 SPF execution for this area."`
}
