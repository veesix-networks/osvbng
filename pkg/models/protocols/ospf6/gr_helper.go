// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf6

type GRHelper struct {
	RouterID             string `json:"routerId"`
	HelperSupport        string `json:"helperSupport"           metric:"label=helper_support"`
	StrictLsaCheck       string `json:"strictLsaCheck"          metric:"label=strict_lsa_check"`
	RestartSupport       string `json:"restartSupport,omitempty"`
	SupportedGracePeriod int    `json:"supportedGracePeriod"    metric:"name=protocols.ospf6.gr_helper.supported_grace_period_seconds,type=gauge,help=OSPFv3 graceful-restart helper grace period in seconds."`
}
