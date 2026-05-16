// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"strings"
	"testing"
)

func TestConfigValidate_NilOrEmpty(t *testing.T) {
	if err := (*Config)(nil).Validate(); err != nil {
		t.Fatalf("nil Config should validate cleanly, got %v", err)
	}
	if err := (&Config{}).Validate(); err != nil {
		t.Fatalf("empty Config should validate cleanly, got %v", err)
	}
}

func TestConfigValidate_PoolsRequireOutsideInterface(t *testing.T) {
	cfg := &Config{
		Pools: map[string]*Pool{
			"default": {Mode: "pba"},
		},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "outside_interface is required") {
		t.Fatalf("expected outside_interface required error, got %v", err)
	}
}

func TestConfigValidate_PoolsWithOutsideInterface(t *testing.T) {
	cfg := &Config{
		OutsideInterface: "eth2",
		Pools: map[string]*Pool{
			"default": {Mode: "pba"},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected pass, got %v", err)
	}
}

func TestConfigValidate_OutsideInterfaceWithoutPools(t *testing.T) {
	cfg := &Config{OutsideInterface: "eth2"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("outside_interface without pools should pass, got %v", err)
	}
}
