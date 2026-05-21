// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf

type GRHelper struct {
	VRF                  string `json:"-"                       metric:"label=vrf,map_key"`
	VRFName              string `json:"vrfName,omitempty"`
	VRFID                int    `json:"vrfId,omitempty"`
	RouterID             string `json:"routerId"`
	HelperSupport        string `json:"helperSupport"           metric:"label=helper_support"`
	StrictLsaCheck       string `json:"strictLsaCheck"          metric:"label=strict_lsa_check"`
	RestartSupport       string `json:"restartSupport,omitempty"`
	SupportedGracePeriod int    `json:"supportedGracePeriod"    metric:"name=protocols.ospf.gr_helper.supported_grace_period_seconds,type=gauge,help=OSPF graceful-restart helper grace period in seconds."`
}
