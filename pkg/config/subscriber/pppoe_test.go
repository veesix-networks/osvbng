// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package subscriber

import "testing"

func TestPPPoEConfigDefaultMRU(t *testing.T) {
	var c *PPPoEConfig
	if got := c.GetMRU(); got != 1492 {
		t.Errorf("nil PPPoEConfig.GetMRU() = %d, want 1492", got)
	}
	c2 := &PPPoEConfig{}
	if got := c2.GetMRU(); got != 1492 {
		t.Errorf("empty PPPoEConfig.GetMRU() = %d, want 1492", got)
	}
}

func TestPPPoEConfigExplicitMRU(t *testing.T) {
	c := &PPPoEConfig{MRU: u16Ptr(1500)}
	if got := c.GetMRU(); got != 1500 {
		t.Errorf("MRU=1500 GetMRU() = %d, want 1500", got)
	}
}

func TestPPPoEConfigIsBabyGiants(t *testing.T) {
	cases := []struct {
		name string
		c    *PPPoEConfig
		want bool
	}{
		{"nil", nil, false},
		{"unset", &PPPoEConfig{}, false},
		{"1492", &PPPoEConfig{MRU: u16Ptr(1492)}, false},
		{"1500", &PPPoEConfig{MRU: u16Ptr(1500)}, true},
		{"1493", &PPPoEConfig{MRU: u16Ptr(1493)}, true},
	}
	for _, tc := range cases {
		if got := tc.c.IsBabyGiants(); got != tc.want {
			t.Errorf("%s: IsBabyGiants() = %v, want %v", tc.name, got, tc.want)
		}
	}
}
