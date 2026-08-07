// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"strings"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/config/l2gw"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
)

func pwTestConfig() *Config {
	return &Config{
		Interfaces: map[string]*interfaces.InterfaceConfig{
			"vxlan-an1": {
				Name:  "vxlan-an1",
				Vxlan: &interfaces.VxlanConfig{Src: "10.0.0.1", Dst: "10.0.0.2", VNI: 10101},
			},
			"pw-an1": {
				Name:       "pw-an1",
				Pseudowire: &interfaces.PseudowireConfig{Transport: "vxlan-an1"},
			},
		},
	}
}

func TestPseudowireValidBinding(t *testing.T) {
	c := pwTestConfig()
	if err := c.validatePseudowires(); err != nil {
		t.Fatalf("expected valid pseudowire config, got: %v", err)
	}
	if !c.Interfaces["vxlan-an1"].Vxlan.PWTransport {
		t.Fatal("transport tunnel not marked as pw transport")
	}
}

func TestPseudowireUnknownTransport(t *testing.T) {
	c := pwTestConfig()
	c.Interfaces["pw-an1"].Pseudowire.Transport = "vxlan-nope"
	err := c.validatePseudowires()
	if err == nil || !strings.Contains(err.Error(), "unknown interface") {
		t.Fatalf("expected unknown transport error, got: %v", err)
	}
}

func TestPseudowireTransportNotTunnel(t *testing.T) {
	c := pwTestConfig()
	c.Interfaces["eth1"] = &interfaces.InterfaceConfig{Name: "eth1"}
	c.Interfaces["pw-an1"].Pseudowire.Transport = "eth1"
	err := c.validatePseudowires()
	if err == nil || !strings.Contains(err.Error(), "not a tunnel") {
		t.Fatalf("expected not-a-tunnel error, got: %v", err)
	}
}

func TestPseudowireTransportDoubleUse(t *testing.T) {
	c := pwTestConfig()
	c.Interfaces["pw-an2"] = &interfaces.InterfaceConfig{
		Name:       "pw-an2",
		Pseudowire: &interfaces.PseudowireConfig{Transport: "vxlan-an1"},
	}
	err := c.validatePseudowires()
	if err == nil || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("expected double-use error, got: %v", err)
	}
}

func TestPseudowireTransportConflictsWithL2GWHandoff(t *testing.T) {
	c := pwTestConfig()
	c.L2GW = &l2gw.L2GWConfig{
		HandoffGroups: map[string]*l2gw.HandoffGroup{
			"isp-blue": {Interface: "vxlan-an1"},
		},
	}
	err := c.validatePseudowires()
	if err == nil || !strings.Contains(err.Error(), "l2gw handoff") {
		t.Fatalf("expected l2gw handoff conflict, got: %v", err)
	}
}

func TestPseudowireTransportConflictsWithSubscriberParent(t *testing.T) {
	c := pwTestConfig()
	c.SubscriberGroups = &subscriber.SubscriberGroupsConfig{
		Groups: map[string]*subscriber.SubscriberGroup{
			"wholesale": {
				VLANs: []subscriber.VLANRange{
					{ParentInterface: "vxlan-an1"},
				},
			},
		},
	}
	err := c.validatePseudowires()
	if err == nil || !strings.Contains(err.Error(), "parent interface") {
		t.Fatalf("expected subscriber parent conflict, got: %v", err)
	}
}

func TestPseudowireInvalidMAC(t *testing.T) {
	c := pwTestConfig()
	c.Interfaces["pw-an1"].Pseudowire.MACAddress = "not-a-mac"
	err := c.validatePseudowires()
	if err == nil || !strings.Contains(err.Error(), "mac-address") {
		t.Fatalf("expected mac validation error, got: %v", err)
	}
}
