// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

type RestartPlan struct {
	NeedsVPP     bool
	NeedsOsvbngd bool
}

func planFromManifest(m *Manifest) RestartPlan {
	var p RestartPlan
	for _, a := range m.Artifacts {
		switch a.RequiresRestart {
		case "vpp":
			p.NeedsVPP = true
		case "osvbngd":
			p.NeedsOsvbngd = true
		case "both":
			p.NeedsVPP = true
			p.NeedsOsvbngd = true
		}
	}
	return p
}
