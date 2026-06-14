// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"strings"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/netbind"
)

func TestBuildListenersNewSchema(t *testing.T) {
	cfg := &Config{Listeners: []ListenerConfig{{Address: ":8080"}}}
	got, err := cfg.buildListeners()
	if err != nil {
		t.Fatalf("buildListeners: %v", err)
	}
	if len(got) != 1 || got[0].Address != ":8080" {
		t.Errorf("got %+v, want one entry :8080", got)
	}
}

func TestBuildListenersLegacyFallback(t *testing.T) {
	cfg := &Config{ListenAddress: ":8443"}
	got, err := cfg.buildListeners()
	if err != nil {
		t.Fatalf("buildListeners: %v", err)
	}
	if len(got) != 1 || got[0].Address != ":8443" {
		t.Errorf("got %+v, want one entry :8443", got)
	}
}

func TestBuildListenersLegacyDefaultPort(t *testing.T) {
	cfg := &Config{ListenerBinding: netbind.ListenerBinding{VRF: "mgmt-vrf"}}
	got, err := cfg.buildListeners()
	if err != nil {
		t.Fatalf("buildListeners: %v", err)
	}
	if len(got) != 1 || got[0].Address != ":8080" || got[0].VRF != "mgmt-vrf" {
		t.Errorf("got %+v, want one entry :8080 vrf=mgmt-vrf", got)
	}
}

func TestBuildListenersAmbiguousRejected(t *testing.T) {
	cfg := &Config{
		Listeners:     []ListenerConfig{{Address: ":8080"}},
		ListenAddress: ":8443",
	}
	_, err := cfg.buildListeners()
	if err == nil {
		t.Fatal("expected ambiguous-config error")
	}
	if !strings.Contains(err.Error(), "remove the deprecated") {
		t.Errorf("error %q does not mention deprecation", err)
	}
}

func TestBuildListenersEmpty(t *testing.T) {
	cfg := &Config{}
	got, err := cfg.buildListeners()
	if err != nil || got != nil {
		t.Errorf("got (%+v, %v), want (nil, nil)", got, err)
	}
}

func TestResolveListenersLogsDeprecationOnLegacy(t *testing.T) {
	cfg := &Config{ListenAddress: ":8080"}
	got, err := cfg.resolveListeners(logger.Get("test"))
	if err != nil {
		t.Fatalf("resolveListeners: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("got %d listeners, want 1", len(got))
	}
}

func TestResolveListenersQuietOnNewSchema(t *testing.T) {
	cfg := &Config{Listeners: []ListenerConfig{{Address: ":8080"}}}
	got, err := cfg.resolveListeners(logger.Get("test"))
	if err != nil {
		t.Fatalf("resolveListeners: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("got %d listeners, want 1", len(got))
	}
}

func TestValidateListenersDuplicateAddress(t *testing.T) {
	cfg := &Config{Listeners: []ListenerConfig{
		{Address: ":8080"},
		{Address: ":8080", ListenerBinding: netbind.ListenerBinding{VRF: "other"}},
	}}
	err := cfg.validateListeners()
	if err == nil {
		t.Fatal("expected duplicate-address error")
	}
	if !strings.Contains(err.Error(), "duplicates") {
		t.Errorf("error %q does not mention duplicate", err)
	}
}

func TestValidateListenersBadAddress(t *testing.T) {
	cfg := &Config{Listeners: []ListenerConfig{{Address: "noport"}}}
	err := cfg.validateListeners()
	if err == nil {
		t.Fatal("expected address-parse error")
	}
}

func TestValidateListenersEmptyAddressRejected(t *testing.T) {
	cfg := &Config{Listeners: []ListenerConfig{{Address: ""}}}
	err := cfg.validateListeners()
	if err == nil {
		t.Fatal("expected required-address error")
	}
}

func TestValidateListenersPartialTLS(t *testing.T) {
	cfg := &Config{Listeners: []ListenerConfig{{
		Address: ":8080",
		TLS:     netbind.ServerTLSConfig{CertFile: "/tmp/x.crt"},
	}}}
	err := cfg.validateListeners()
	if err == nil {
		t.Fatal("expected partial-TLS error")
	}
}

func TestValidateListenersEmptyOK(t *testing.T) {
	cfg := &Config{}
	if err := cfg.validateListeners(); err != nil {
		t.Errorf("empty config should validate, got %v", err)
	}
}
