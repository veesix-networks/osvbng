// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package subscriber

const DefaultPPPMRU uint16 = 1492

type PPPoEConfig struct {
	MRU *uint16 `json:"mru,omitempty" yaml:"mru,omitempty"`
}

func (c *PPPoEConfig) GetMRU() uint16 {
	if c == nil || c.MRU == nil {
		return DefaultPPPMRU
	}
	return *c.MRU
}

func (c *PPPoEConfig) IsBabyGiants() bool {
	return c.GetMRU() > DefaultPPPMRU
}
