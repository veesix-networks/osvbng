// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import "testing"

func TestPlanFromManifestAggregates(t *testing.T) {
	cases := map[string]struct {
		artifacts []string
		want      RestartPlan
	}{
		"empty": {
			artifacts: nil,
			want:      RestartPlan{},
		},
		"only none": {
			artifacts: []string{"none"},
			want:      RestartPlan{},
		},
		"single osvbngd": {
			artifacts: []string{"osvbngd"},
			want:      RestartPlan{NeedsOsvbngd: true},
		},
		"single vpp": {
			artifacts: []string{"vpp"},
			want:      RestartPlan{NeedsVPP: true},
		},
		"single both": {
			artifacts: []string{"both"},
			want:      RestartPlan{NeedsVPP: true, NeedsOsvbngd: true},
		},
		"vpp plus osvbngd equals both": {
			artifacts: []string{"vpp", "osvbngd"},
			want:      RestartPlan{NeedsVPP: true, NeedsOsvbngd: true},
		},
		"both plus none unchanged": {
			artifacts: []string{"both", "none", "none"},
			want:      RestartPlan{NeedsVPP: true, NeedsOsvbngd: true},
		},
	}

	for name, c := range cases {
		c := c
		t.Run(name, func(t *testing.T) {
			m := &Manifest{}
			for _, rr := range c.artifacts {
				m.Artifacts = append(m.Artifacts, ManifestArtifact{RequiresRestart: rr})
			}
			got := planFromManifest(m)
			if got != c.want {
				t.Fatalf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestPlanFromManifestBothEqualsOsvbngdPlusVPP(t *testing.T) {
	combined := planFromManifest(&Manifest{
		Artifacts: []ManifestArtifact{
			{RequiresRestart: "vpp"},
			{RequiresRestart: "osvbngd"},
		},
	})
	bothOnly := planFromManifest(&Manifest{
		Artifacts: []ManifestArtifact{
			{RequiresRestart: "both"},
		},
	})
	if combined != bothOnly {
		t.Fatalf("combined %+v != bothOnly %+v", combined, bothOnly)
	}
}
