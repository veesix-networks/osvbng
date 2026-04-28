// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package radius

import (
	"testing"

	"github.com/veesix-networks/osvbng/pkg/netbind"
)

func TestServerEffectiveBinding_GroupDefault(t *testing.T) {
	cfg := &Config{
		EndpointBinding: netbind.EndpointBinding{VRF: "MGMT-VRF", SourceIP: "10.99.0.254"},
		Servers: []ServerConfig{
			{Host: "10.99.0.1", Secret: "s"},
		},
	}

	effective := cfg.Servers[0].EndpointBinding.MergeWith(cfg.EndpointBinding)
	if effective.VRF != "MGMT-VRF" {
		t.Errorf("VRF=%q want MGMT-VRF", effective.VRF)
	}
	if effective.SourceIP != "10.99.0.254" {
		t.Errorf("SourceIP=%q want 10.99.0.254", effective.SourceIP)
	}
}

func TestServerEffectiveBinding_PerServerOverride(t *testing.T) {
	cfg := &Config{
		EndpointBinding: netbind.EndpointBinding{VRF: "MGMT-VRF", SourceIP: "10.99.0.254"},
		Servers: []ServerConfig{
			{
				EndpointBinding: netbind.EndpointBinding{VRF: "OOB-VRF", SourceIP: "10.50.0.254"},
				Host:            "10.50.0.1",
				Secret:          "s",
			},
		},
	}

	effective := cfg.Servers[0].EndpointBinding.MergeWith(cfg.EndpointBinding)
	if effective.VRF != "OOB-VRF" {
		t.Errorf("VRF=%q want OOB-VRF", effective.VRF)
	}
	if effective.SourceIP != "10.50.0.254" {
		t.Errorf("SourceIP=%q want 10.50.0.254", effective.SourceIP)
	}
}

func TestServerEffectiveBinding_PartialOverride(t *testing.T) {
	cfg := &Config{
		EndpointBinding: netbind.EndpointBinding{VRF: "MGMT-VRF", SourceIP: "10.99.0.254"},
		Servers: []ServerConfig{
			{
				EndpointBinding: netbind.EndpointBinding{SourceIP: "10.99.0.99"},
				Host:            "10.99.0.2",
				Secret:          "s",
			},
		},
	}

	effective := cfg.Servers[0].EndpointBinding.MergeWith(cfg.EndpointBinding)
	if effective.VRF != "MGMT-VRF" {
		t.Errorf("VRF=%q want inherited MGMT-VRF", effective.VRF)
	}
	if effective.SourceIP != "10.99.0.99" {
		t.Errorf("SourceIP=%q want 10.99.0.99", effective.SourceIP)
	}
}

func TestServerEffectiveBinding_ResolvesToBinding(t *testing.T) {
	cfg := &Config{
		EndpointBinding: netbind.EndpointBinding{VRF: "MGMT-VRF", SourceIP: "10.99.0.254"},
		Servers: []ServerConfig{
			{Host: "10.99.0.1", Secret: "s"},
		},
	}

	effective := cfg.Servers[0].EndpointBinding.MergeWith(cfg.EndpointBinding)
	bind, err := effective.Resolve(netbind.FamilyV4)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if bind.VRF != "MGMT-VRF" {
		t.Errorf("Binding.VRF=%q want MGMT-VRF", bind.VRF)
	}
	if bind.SourceIP.String() != "10.99.0.254" {
		t.Errorf("Binding.SourceIP=%v want 10.99.0.254", bind.SourceIP)
	}
}

func TestServerFamily(t *testing.T) {
	cases := []struct {
		host string
		want netbind.Family
	}{
		{"10.0.0.1", netbind.FamilyV4},
		{"10.0.0.1:1812", netbind.FamilyV4},
		{"2001:db8::1", netbind.FamilyV6},
		{"[2001:db8::1]:1812", netbind.FamilyV6},
		{"radius.example.com", netbind.FamilyV4},
		{"radius.example.com:1812", netbind.FamilyV4},
	}
	for _, c := range cases {
		t.Run(c.host, func(t *testing.T) {
			if got := serverFamily(c.host); got != c.want {
				t.Errorf("serverFamily(%q)=%v want %v", c.host, got, c.want)
			}
		})
	}
}

func TestCoAClientFamily(t *testing.T) {
	cases := []struct {
		host string
		want netbind.Family
	}{
		{"10.99.0.50", netbind.FamilyV4},
		{"10.99.0.0/24", netbind.FamilyV4},
		{"2001:db8::50", netbind.FamilyV6},
		{"2001:db8::/64", netbind.FamilyV6},
	}
	for _, c := range cases {
		t.Run(c.host, func(t *testing.T) {
			if got := coaClientFamily(c.host); got != c.want {
				t.Errorf("coaClientFamily(%q)=%v want %v", c.host, got, c.want)
			}
		})
	}
}

func TestApplyDefaults_CoAListenerPort(t *testing.T) {
	cfg := &Config{}
	cfg.applyDefaults()
	if cfg.CoAListener.Port != DefaultCoAPort {
		t.Errorf("CoAListener.Port=%d want %d", cfg.CoAListener.Port, DefaultCoAPort)
	}
}

func TestApplyDefaults_CoAListenerPortRespectsConfigured(t *testing.T) {
	cfg := &Config{
		CoAListener: CoAListenerConfig{Port: 4242},
	}
	cfg.applyDefaults()
	if cfg.CoAListener.Port != 4242 {
		t.Errorf("CoAListener.Port=%d want 4242 (operator-set)", cfg.CoAListener.Port)
	}
}
