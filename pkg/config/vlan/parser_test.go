// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package vlan

import "testing"

func TestParseCVLAN(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantAny bool
		wantID  uint16
		wantErr bool
	}{
		{name: "empty is any", in: "", wantAny: true},
		{name: "whitespace is any", in: "  ", wantAny: true},
		{name: "any", in: "any", wantAny: true},
		{name: "any mixed case", in: "Any", wantAny: true},
		{name: "specific", in: "200", wantID: 200},
		{name: "zero rejected", in: "0", wantErr: true},
		{name: "over max rejected", in: "4095", wantErr: true},
		{name: "non-numeric rejected", in: "none", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isAny, id, err := ParseCVLAN(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseCVLAN(%q): expected error, got none", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseCVLAN(%q): unexpected error: %v", tt.in, err)
			}
			if isAny != tt.wantAny {
				t.Errorf("ParseCVLAN(%q): isAny=%v, want %v", tt.in, isAny, tt.wantAny)
			}
			if !isAny && id != tt.wantID {
				t.Errorf("ParseCVLAN(%q): id=%d, want %d", tt.in, id, tt.wantID)
			}
		})
	}
}
