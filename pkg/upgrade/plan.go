// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

// RestartPlan summarises the per-component restart needs implied by a
// manifest's artifacts. NeedsVPP drives the dataplane-restart code path
// in the runner. NeedsOsvbngd is informational only: every apply
// restarts the daemon as part of the always-stop invariant, so the field
// is surfaced in `upgrade plan` output but does not change runner
// behaviour.
type RestartPlan struct {
	NeedsVPP     bool
	NeedsOsvbngd bool
}

// planFromManifest derives the aggregate restart plan from the
// per-artifact requires_restart values. Validate has already enforced
// that every artifact carries one of the four enum values, so an
// unknown value here would be a programming error rather than user
// input.
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
		case "none":
			// no-op
		}
	}
	return p
}
