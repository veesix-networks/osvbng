// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"strings"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/netbind"
)

func haConfigForSRGTest(srgs map[string]*SRGConfig) *HAConfig {
	return &HAConfig{
		Enabled: true,
		NodeID:  "node1",
		Peer:    HAPeerConfig{Address: "10.0.0.2:50051"},
		SRGs:    srgs,
	}
}

func nopVRFLookup(string) (netbind.VRFInfo, bool) {
	return netbind.VRFInfo{}, true
}

func TestHAValidate_SRGVirtualMACRequiredWithSubscriberGroups(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		srg      *SRGConfig
		wantErr  bool
		errMatch string
	}{
		{
			name: "subscriber_groups_with_virtual_mac_accepts",
			srg: &SRGConfig{
				Priority:         100,
				VirtualMAC:       "02:00:00:00:00:01",
				SubscriberGroups: []string{"access1"},
			},
			wantErr: false,
		},
		{
			name: "subscriber_groups_without_virtual_mac_rejects",
			srg: &SRGConfig{
				Priority:         100,
				SubscriberGroups: []string{"access1"},
			},
			wantErr:  true,
			errMatch: "virtual_mac: required when subscriber_groups is non-empty",
		},
		{
			name: "subscriber_groups_with_invalid_virtual_mac_rejects",
			srg: &SRGConfig{
				Priority:         100,
				VirtualMAC:       "not-a-mac",
				SubscriberGroups: []string{"access1"},
			},
			wantErr:  true,
			errMatch: "virtual_mac",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := haConfigForSRGTest(map[string]*SRGConfig{"access1": tt.srg})
			err := cfg.Validate(nopVRFLookup)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error matching %q, got nil", tt.errMatch)
				}
				if !strings.Contains(err.Error(), tt.errMatch) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.errMatch)
				}
				return
			}

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func TestHAValidate_EmptySubscriberGroupsRejected(t *testing.T) {
	t.Parallel()

	cfg := haConfigForSRGTest(map[string]*SRGConfig{
		"orphan": {
			Priority:   100,
			VirtualMAC: "02:00:00:00:00:01",
		},
	})
	err := cfg.Validate(nopVRFLookup)
	if err == nil {
		t.Fatalf("expected subscriber_groups required error, got nil")
	}
	if !strings.Contains(err.Error(), "subscriber_groups") {
		t.Fatalf("error %q does not mention subscriber_groups", err.Error())
	}
}
