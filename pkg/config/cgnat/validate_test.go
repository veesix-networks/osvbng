// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"strings"
	"testing"
)

func poolWith(mutate func(*Pool)) *Pool {
	p := &Pool{
		Mode:              "pba",
		OutsideInterfaces: []string{"eth2"},
	}
	mutate(p)
	return p
}

func validatePool(p *Pool) error {
	return (&Config{Pools: map[string]*Pool{"residential": p}}).Validate()
}

func expectRefused(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected the config to be refused, it validated cleanly")
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("expected an error mentioning %q, got %v", want, err)
	}
	if !strings.Contains(err.Error(), `"residential"`) && !strings.Contains(err.Error(), "pools") {
		t.Fatalf("expected the pool name in the error, got %v", err)
	}
}

// The reported boot loop: the block size truncates to zero and ConfigurePool
// divides the port range by it inside Component.Start, which has no recover.
func TestConfigValidate_SubscriberRatioWiderThanPortRange(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ratio uint16
		ports string
	}{
		{"default range", 65535, ""},
		{"narrow range", 2000, "1024-2047"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := poolWith(func(p *Pool) {
				p.SubscriberRatio = tc.ratio
				p.PortRange = tc.ports
			})
			if got := p.GetBlockSize(); got != 0 {
				t.Fatalf("fixture no longer reproduces a zero block size, got %d", got)
			}
			expectRefused(t, validatePool(p), "block size computes to 0")
		})
	}
}

func TestConfigValidate_BlockSizeWiderThanPortRange(t *testing.T) {
	p := poolWith(func(p *Pool) {
		p.BlockSize = 4096
		p.PortRange = "1024-2047"
	})
	expectRefused(t, validatePool(p), "block-size 4096 exceeds")
}

func TestConfigValidate_DefaultBlockSizeWiderThanPortRange(t *testing.T) {
	p := poolWith(func(p *Pool) { p.PortRange = "1024-1200" })
	expectRefused(t, validatePool(p), "default block-size")
}

func TestConfigValidate_InvertedPortRange(t *testing.T) {
	p := poolWith(func(p *Pool) { p.PortRange = "9000-1024" })
	expectRefused(t, validatePool(p), "end 1024 is below start 9000")
}

func TestConfigValidate_MalformedPortRange(t *testing.T) {
	for _, spec := range []string{"1024", "", "1024-70000", "abc-def", "0-1024"} {
		p := poolWith(func(p *Pool) { p.PortRange = spec })
		if spec == "" {
			// An unset range is the default, not a fault.
			if err := validatePool(p); err != nil {
				t.Fatalf("unset port-range should validate, got %v", err)
			}
			continue
		}
		if err := validatePool(p); err == nil {
			t.Fatalf("port-range %q should be refused", spec)
		}
	}
}

// A typo used to select the default silently: "determinstic" ran as PBA.
func TestConfigValidate_EnumTypos(t *testing.T) {
	for _, tc := range []struct {
		field  string
		mutate func(*Pool)
	}{
		{"mode", func(p *Pool) { p.Mode = "determinstic" }},
		{"filtering", func(p *Pool) { p.Filtering = "endpoint-indepedent" }},
		{"address-pooling", func(p *Pool) { p.AddressPooling = "pared" }},
	} {
		t.Run(tc.field, func(t *testing.T) {
			expectRefused(t, validatePool(poolWith(tc.mutate)), tc.field+" must be one of")
		})
	}
}

func TestConfigValidate_EnumValuesTheDataplaneImplements(t *testing.T) {
	p := poolWith(func(p *Pool) {
		p.Mode = "deterministic"
		p.Filtering = "endpoint-dependent"
		p.AddressPooling = "arbitrary"
	})
	if err := validatePool(p); err != nil {
		t.Fatalf("expected the implemented spellings to validate, got %v", err)
	}
}

func TestConfigValidate_BadInsidePrefix(t *testing.T) {
	p := poolWith(func(p *Pool) {
		p.InsidePrefixes = []InsidePrefix{{Prefix: "100.64.0.0"}}
	})
	expectRefused(t, validatePool(p), "is not an IPv4 CIDR")
}

// One allocator entry per address: a /8 does not survive boot, and /0
// additionally overflows the host count to zero addresses.
func TestConfigValidate_OutsidePrefixTooWide(t *testing.T) {
	for _, spec := range []string{"0.0.0.0/0", "10.0.0.0/8", "172.16.0.0/12"} {
		p := poolWith(func(p *Pool) { p.OutsideAddresses = []string{spec} })
		expectRefused(t, validatePool(p), "is wider than /16")
	}
}

func TestConfigValidate_OutsidePrefixAtTheLimit(t *testing.T) {
	for _, spec := range []string{"198.51.0.0/16", "203.0.113.0/28", "203.0.113.5"} {
		p := poolWith(func(p *Pool) { p.OutsideAddresses = []string{spec} })
		if err := validatePool(p); err != nil {
			t.Fatalf("outside prefix %s should validate, got %v", spec, err)
		}
	}
}

func TestConfigValidate_BadOutsideAddress(t *testing.T) {
	p := poolWith(func(p *Pool) { p.OutsideAddresses = []string{"203.0.113.256"} })
	expectRefused(t, validatePool(p), "is not an IPv4 address or CIDR")
}

func TestConfigValidate_BadExcludedAddress(t *testing.T) {
	p := poolWith(func(p *Pool) { p.ExcludedAddresses = []string{"203.0.113.0/28"} })
	expectRefused(t, validatePool(p), "must be a single address or a /32")
}

// The allocator compares an exclusion against net.IP.String(); both spellings
// have to reduce to that or the entry silently matches nothing.
func TestParseExcludedAddress_BothSpellingsNormalise(t *testing.T) {
	for _, spec := range []string{"10.0.0.1", "10.0.0.1/32"} {
		ip, err := ParseExcludedAddress(spec)
		if err != nil {
			t.Fatalf("%q should parse, got %v", spec, err)
		}
		if ip.String() != "10.0.0.1" {
			t.Fatalf("%q normalised to %q, want 10.0.0.1", spec, ip.String())
		}
	}
}

// FindPoolForIP walks the pool map in random order, so two pools claiming one
// address answer differently per call.
func TestConfigValidate_OverlappingInsidePrefixesAcrossPools(t *testing.T) {
	cfg := &Config{Pools: map[string]*Pool{
		"ispA": poolWith(func(p *Pool) {
			p.InsidePrefixes = []InsidePrefix{{Prefix: "100.64.0.0/16"}}
		}),
		"ispB": poolWith(func(p *Pool) {
			p.InsidePrefixes = []InsidePrefix{{Prefix: "100.64.1.0/24"}}
		}),
	}}
	expectRefused(t, cfg.Validate(), "both claim inside prefix")
}

func TestConfigValidate_OverlappingInsidePrefixesNeedACommonVRF(t *testing.T) {
	cfg := &Config{Pools: map[string]*Pool{
		"ispA": poolWith(func(p *Pool) {
			p.InsidePrefixes = []InsidePrefix{{Prefix: "100.64.0.0/16", VRF: "CUSTOMER-A"}}
		}),
		"ispB": poolWith(func(p *Pool) {
			p.InsidePrefixes = []InsidePrefix{{Prefix: "100.64.0.0/16", VRF: "CUSTOMER-B"}}
		}),
	}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("the same prefix in two VRFs is not ambiguous, got %v", err)
	}
}

// An unset VRF matches any VRF at classification, so it collides with a
// named one.
func TestConfigValidate_UnsetVRFCollidesWithANamedOne(t *testing.T) {
	cfg := &Config{Pools: map[string]*Pool{
		"ispA": poolWith(func(p *Pool) {
			p.InsidePrefixes = []InsidePrefix{{Prefix: "100.64.0.0/16"}}
		}),
		"ispB": poolWith(func(p *Pool) {
			p.InsidePrefixes = []InsidePrefix{{Prefix: "100.64.0.0/16", VRF: "CUSTOMER-A"}}
		}),
	}}
	expectRefused(t, cfg.Validate(), "both claim inside prefix")
}

// One pool carrying the same prefix in several VRFs is the wholesale shape
// the VRF rig suite ships.
func TestConfigValidate_SamePrefixTwiceInOnePool(t *testing.T) {
	cfg := &Config{Pools: map[string]*Pool{
		"residential": poolWith(func(p *Pool) {
			p.InsidePrefixes = []InsidePrefix{
				{Prefix: "100.64.0.0/24", VRF: "CUSTOMER-A"},
				{Prefix: "100.64.0.0/24", VRF: "CUSTOMER-B"},
			}
			p.OutsideAddresses = []string{"203.0.113.0/28"}
		}),
	}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected the shipped VRF pool shape to validate, got %v", err)
	}
}

func TestConfigValidate_OverlappingOutsideAddressesAcrossPools(t *testing.T) {
	cfg := &Config{Pools: map[string]*Pool{
		"ispA": poolWith(func(p *Pool) { p.OutsideAddresses = []string{"203.0.113.0/28"} }),
		"ispB": poolWith(func(p *Pool) { p.OutsideAddresses = []string{"203.0.113.4"} }),
	}}
	expectRefused(t, cfg.Validate(), "both claim outside address")
}

// The pool every CGNAT rig suite ships, so a refusal here would be a
// regression against a config known to pass traffic.
func TestConfigValidate_ShippedRigPoolValidates(t *testing.T) {
	cfg := &Config{Pools: map[string]*Pool{
		"residential": {
			OutsideInterfaces:        []string{"eth2"},
			Mode:                     "pba",
			InsidePrefixes:           []InsidePrefix{{Prefix: "100.64.0.0/16"}},
			OutsideAddresses:         []string{"203.0.113.0/28"},
			BlockSize:                512,
			MaxBlocksPerSubscriber:   2,
			MaxSessionsPerSubscriber: 2000,
			AddressPooling:           "paired",
			Filtering:                "endpoint-independent",
			Timeouts: &TimeoutConfig{
				TCPEstablished: 7200,
				TCPTransitory:  240,
				UDP:            300,
				ICMP:           60,
			},
		},
	}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected the shipped rig pool to validate, got %v", err)
	}
}

// The wholesale example in docs/configuration/cgnat.md: one pool per ISP,
// the same inside prefix in each ISP's own VRF, separate outside space.
func TestConfigValidate_DocumentedWholesaleExample(t *testing.T) {
	cfg := &Config{Pools: map[string]*Pool{
		"ispA": {
			OutsideInterfaces: []string{"bond0.100", "bond0.101"},
			Mode:              "pba",
			InsidePrefixes:    []InsidePrefix{{Prefix: "100.64.0.0/16", VRF: "ispA-inside"}},
			OutsideAddresses:  []string{"203.0.113.0/24"},
		},
		"ispB": {
			OutsideInterfaces: []string{"bond0.200", "bond0.201"},
			Mode:              "pba",
			InsidePrefixes:    []InsidePrefix{{Prefix: "100.64.0.0/16", VRF: "ispB-inside"}},
			OutsideAddresses:  []string{"198.51.100.0/24"},
		},
	}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected the documented wholesale example to validate, got %v", err)
	}
}
