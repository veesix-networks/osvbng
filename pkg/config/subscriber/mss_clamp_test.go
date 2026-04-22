// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package subscriber

import "testing"

func boolPtr(b bool) *bool    { return &b }
func u16Ptr(v uint16) *uint16 { return &v }

func TestMSSClampDefaultEnabled(t *testing.T) {
	var c *MSSClampConfig
	if !c.IsEnabled() {
		t.Error("nil MSSClampConfig should be enabled by default")
	}
	c2 := &MSSClampConfig{}
	if !c2.IsEnabled() {
		t.Error("MSSClampConfig with no Enabled set should be enabled by default")
	}
}

func TestMSSClampExplicitDisabled(t *testing.T) {
	c := &MSSClampConfig{Enabled: boolPtr(false)}
	if c.IsEnabled() {
		t.Error("explicit Enabled=false should be disabled")
	}
}

func TestSubscriberPathMTUDefault(t *testing.T) {
	var c *MSSClampConfig
	if got := c.GetSubscriberPathMTU(); got != 1500 {
		t.Errorf("nil GetSubscriberPathMTU() = %d, want 1500", got)
	}
	c2 := &MSSClampConfig{}
	if got := c2.GetSubscriberPathMTU(); got != 1500 {
		t.Errorf("unset GetSubscriberPathMTU() = %d, want 1500", got)
	}
}

func TestSubscriberPathMTUOverride(t *testing.T) {
	c := &MSSClampConfig{SubscriberPathMTU: u16Ptr(9000)}
	if got := c.GetSubscriberPathMTU(); got != 9000 {
		t.Errorf("operator override GetSubscriberPathMTU() = %d, want 9000", got)
	}
}

func TestMSSClampAutoFromSubscriberPathMTUDefault(t *testing.T) {
	c := &MSSClampConfig{}
	if got := c.IPv4MSSAuto(); got != 1460 {
		t.Errorf("default subscriber path MTU IPv4 auto = %d, want 1460", got)
	}
	if got := c.IPv6MSSAuto(); got != 1440 {
		t.Errorf("default subscriber path MTU IPv6 auto = %d, want 1440", got)
	}
}

func TestMSSClampAutoFromJumboSubscriberPathMTU(t *testing.T) {
	c := &MSSClampConfig{SubscriberPathMTU: u16Ptr(9000)}
	if got := c.IPv4MSSAuto(); got != 8960 {
		t.Errorf("jumbo subscriber path MTU IPv4 auto = %d, want 8960", got)
	}
	if got := c.IPv6MSSAuto(); got != 8940 {
		t.Errorf("jumbo subscriber path MTU IPv6 auto = %d, want 8940", got)
	}
}

func TestMSSClampAutoExplicitOverride(t *testing.T) {
	c := &MSSClampConfig{
		SubscriberPathMTU: u16Ptr(9000),
		IPv4MSS:           u16Ptr(1400),
		IPv6MSS:           u16Ptr(1380),
	}
	if got := c.IPv4MSSAuto(); got != 1400 {
		t.Errorf("explicit IPv4MSS = %d, want 1400 (must beat subscriber-path-mtu)", got)
	}
	if got := c.IPv6MSSAuto(); got != 1380 {
		t.Errorf("explicit IPv6MSS = %d, want 1380 (must beat subscriber-path-mtu)", got)
	}
}

func TestMSSClampOrAutoForPPPoE(t *testing.T) {
	c := &MSSClampConfig{}
	if got := c.IPv4MSSOrAuto(1492); got != 1452 {
		t.Errorf("PPPoE default 1492 IPv4 = %d, want 1452", got)
	}
	if got := c.IPv4MSSOrAuto(1500); got != 1460 {
		t.Errorf("PPPoE baby-giants 1500 IPv4 = %d, want 1460", got)
	}
	if got := c.IPv6MSSOrAuto(1492); got != 1432 {
		t.Errorf("PPPoE default 1492 IPv6 = %d, want 1432", got)
	}
	if got := c.IPv6MSSOrAuto(1500); got != 1440 {
		t.Errorf("PPPoE baby-giants 1500 IPv6 = %d, want 1440", got)
	}
}

func TestMSSClampOrAutoOverrideWins(t *testing.T) {
	c := &MSSClampConfig{
		IPv4MSS: u16Ptr(1400),
		IPv6MSS: u16Ptr(1380),
	}
	if got := c.IPv4MSSOrAuto(1500); got != 1400 {
		t.Errorf("explicit IPv4MSS = %d, want 1400 (must beat per-session mtu)", got)
	}
	if got := c.IPv6MSSOrAuto(1500); got != 1380 {
		t.Errorf("explicit IPv6MSS = %d, want 1380 (must beat per-session mtu)", got)
	}
}

