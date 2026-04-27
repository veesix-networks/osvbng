// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package netbind

import (
	"errors"
	"fmt"
	"net/netip"
)

type ListenerBinding struct {
	VRF string `json:"vrf,omitempty" yaml:"vrf,omitempty"`
}

func (b ListenerBinding) IsZero() bool { return b.VRF == "" }

func (b ListenerBinding) Resolve() Binding { return Binding{VRF: b.VRF} }

func (b ListenerBinding) Validate(family Family, vrfMgr VRFResolver, nl LinkLister) error {
	if b.IsZero() {
		return nil
	}
	return ValidateBinding(vrfMgr, nl, b.Resolve(), family)
}

type EndpointBinding struct {
	VRF             string `json:"vrf,omitempty"              yaml:"vrf,omitempty"`
	SourceIP        string `json:"source_ip,omitempty"        yaml:"source_ip,omitempty"`
	SourceIPv6      string `json:"source_ipv6,omitempty"      yaml:"source_ipv6,omitempty"`
	SourceInterface string `json:"source_interface,omitempty" yaml:"source_interface,omitempty"`
}

func (b EndpointBinding) IsZero() bool {
	return b.VRF == "" && b.SourceIP == "" && b.SourceIPv6 == "" && b.SourceInterface == ""
}

// MergeWith fills empty fields in b from parent. Field-by-field, so the
// caller can override a single field without losing inherited siblings.
func (b EndpointBinding) MergeWith(parent EndpointBinding) EndpointBinding {
	out := b
	if out.VRF == "" {
		out.VRF = parent.VRF
	}
	if out.SourceIP == "" {
		out.SourceIP = parent.SourceIP
	}
	if out.SourceIPv6 == "" {
		out.SourceIPv6 = parent.SourceIPv6
	}
	if out.SourceInterface == "" {
		out.SourceInterface = parent.SourceInterface
	}
	return out
}

// Resolve parses the strings into a runtime Binding for family. nl is
// reserved for future SourceInterface resolution.
func (b EndpointBinding) Resolve(family Family, _ LinkLister) (Binding, error) {
	if b.SourceInterface != "" {
		return Binding{}, errors.New("netbind: source_interface not yet implemented; use source_ip / source_ipv6")
	}

	out := Binding{VRF: b.VRF}

	src, err := b.sourceIPForFamily(family)
	if err != nil {
		return Binding{}, err
	}
	out.SourceIP = src

	return out, nil
}

func (b EndpointBinding) Validate(family Family, vrfMgr VRFResolver, nl LinkLister) error {
	if b.IsZero() {
		return nil
	}
	if b.SourceInterface != "" {
		return errors.New("netbind: source_interface not yet implemented; use source_ip / source_ipv6")
	}
	runtime, err := b.Resolve(family, nl)
	if err != nil {
		return err
	}
	return ValidateBinding(vrfMgr, nl, runtime, family)
}

func (b EndpointBinding) sourceIPForFamily(family Family) (netip.Addr, error) {
	var raw string
	switch family {
	case FamilyV4:
		raw = b.SourceIP
	case FamilyV6:
		raw = b.SourceIPv6
	default:
		return netip.Addr{}, fmt.Errorf("netbind: invalid family %d", family)
	}
	if raw == "" {
		return netip.Addr{}, nil
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("netbind: source IP %q: %w", raw, err)
	}
	switch family {
	case FamilyV4:
		if !addr.Is4() && !addr.Is4In6() {
			return netip.Addr{}, fmt.Errorf("netbind: source_ip %s is IPv6, expected IPv4", addr)
		}
	case FamilyV6:
		if addr.Is4() {
			return netip.Addr{}, fmt.Errorf("netbind: source_ipv6 %s is IPv4, expected IPv6", addr)
		}
	}
	return addr, nil
}
