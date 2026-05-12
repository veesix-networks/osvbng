// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package isis

type Area struct {
	Area     string    `json:"area"     metric:"label=area"`
	Circuits []Circuit `json:"circuits" metric:"flatten"`
}

type Circuit struct {
	Circuit   int    `json:"circuit"             metric:"label=circuit"`
	Adj       string `json:"adj,omitempty"`
	Interface string `json:"interface,omitempty" metric:"label"`
	Level     int    `json:"level,omitempty"     metric:"name=protocols.isis.circuit.level,type=gauge,help=ISIS level for this circuit."`
	State     string `json:"state,omitempty"     metric:"label"`
	ExpiresIn string `json:"expires-in,omitempty"`
	SNPA      string `json:"snpa,omitempty"`
}
