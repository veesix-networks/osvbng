// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ip

import "testing"

func TestIPv6RAConfigGetOnLink(t *testing.T) {
	t.Parallel()

	tr := true
	fa := false

	tests := []struct {
		name string
		cfg  *IPv6RAConfig
		want bool
	}{
		{name: "nil_receiver_defaults_off_link", cfg: nil, want: false},
		{name: "unset_defaults_off_link", cfg: &IPv6RAConfig{}, want: false},
		{name: "explicit_true", cfg: &IPv6RAConfig{OnLink: &tr}, want: true},
		{name: "explicit_false", cfg: &IPv6RAConfig{OnLink: &fa}, want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.cfg.GetOnLink(); got != tt.want {
				t.Fatalf("GetOnLink() = %v, want %v", got, tt.want)
			}
		})
	}
}
