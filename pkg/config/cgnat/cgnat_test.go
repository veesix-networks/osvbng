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

func TestConfigValidate_PoolRequiresOutsideInterfaces(t *testing.T) {
	cfg := &Config{
		Pools: map[string]*Pool{
			"residential": {Mode: "pba"},
		},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "outside_interfaces is required") {
		t.Fatalf("expected per-pool outside_interfaces required error, got %v", err)
	}
	if !strings.Contains(err.Error(), `pool "residential"`) {
		t.Fatalf("expected pool name in error, got %v", err)
	}
}

func TestConfigValidate_PoolOutsideInterfacesSingleEntry(t *testing.T) {
	cfg := &Config{
		Pools: map[string]*Pool{
			"residential": {
				Mode:              "pba",
				OutsideInterfaces: []string{"eth2"},
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected pass, got %v", err)
	}
}

func TestConfigValidate_PoolOutsideInterfacesMultipleEntries(t *testing.T) {
	cfg := &Config{
		Pools: map[string]*Pool{
			"residential": {
				Mode:              "pba",
				OutsideInterfaces: []string{"bond0.100", "bond0.101"},
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected pass, got %v", err)
	}
}

func TestConfigValidate_PoolOutsideInterfacesEmptyEntry(t *testing.T) {
	cfg := &Config{
		Pools: map[string]*Pool{
			"residential": {
				Mode:              "pba",
				OutsideInterfaces: []string{"eth2", ""},
			},
		},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "empty interface name") {
		t.Fatalf("expected empty interface name error, got %v", err)
	}
}

func TestConfigValidate_PoolOutsideInterfacesDuplicate(t *testing.T) {
	cfg := &Config{
		Pools: map[string]*Pool{
			"residential": {
				Mode:              "pba",
				OutsideInterfaces: []string{"bond0.100", "bond0.100"},
			},
		},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), `duplicate entry "bond0.100"`) {
		t.Fatalf("expected duplicate entry error, got %v", err)
	}
}

func TestConfigValidate_LegacyTopLevelOutsideInterfaceFails(t *testing.T) {
	legacy := "eth2"
	cfg := &Config{
		LegacyOutsideInterface: &legacy,
		Pools: map[string]*Pool{
			"residential": {Mode: "pba", OutsideInterfaces: []string{"eth2"}},
		},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "per-pool outside_interfaces") {
		t.Fatalf("expected legacy migration error, got %v", err)
	}
}

func TestConfigValidate_LegacyTopLevelOutsideInterfacesListFails(t *testing.T) {
	cfg := &Config{
		LegacyOutsideInterfaces: []string{"eth2"},
		Pools: map[string]*Pool{
			"residential": {Mode: "pba", OutsideInterfaces: []string{"eth2"}},
		},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "moved into each pool block") {
		t.Fatalf("expected legacy top-level migration error, got %v", err)
	}
}

func TestConfigValidate_WholesaleTwoPoolsTwoVRFs(t *testing.T) {
	cfg := &Config{
		Pools: map[string]*Pool{
			"ispA": {
				Mode:              "pba",
				OutsideInterfaces: []string{"bond0.100", "bond0.101"},
			},
			"ispB": {
				Mode:              "pba",
				OutsideInterfaces: []string{"bond0.200", "bond0.201"},
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected wholesale config to pass schema validation, got %v", err)
	}
}
