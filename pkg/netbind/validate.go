// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package netbind

import (
	"fmt"
	"net/netip"
)

type VRFInfo struct {
	IPv4 bool
	IPv6 bool
}

type VRFLookup func(name string) (VRFInfo, bool)

func (b Binding) Validate(family Family, lookup VRFLookup) error {
	if b.IsZero() {
		return nil
	}
	if family != FamilyV4 && family != FamilyV6 {
		return fmt.Errorf("netbind: invalid family %d", family)
	}
	if err := validateSourceIPFamily(b.SourceIP, family); err != nil {
		return err
	}
	if b.VRF == "" {
		return nil
	}
	if lookup == nil {
		return fmt.Errorf("netbind: vrf %q referenced but no VRF source available", b.VRF)
	}
	info, ok := lookup(b.VRF)
	if !ok {
		return fmt.Errorf("netbind: vrf %q not declared", b.VRF)
	}
	switch family {
	case FamilyV4:
		if !info.IPv4 {
			return fmt.Errorf("netbind: vrf %q does not have ipv4-unicast enabled", b.VRF)
		}
	case FamilyV6:
		if !info.IPv6 {
			return fmt.Errorf("netbind: vrf %q does not have ipv6-unicast enabled", b.VRF)
		}
	}
	return nil
}

func validateSourceIPFamily(src netip.Addr, family Family) error {
	if !src.IsValid() {
		return nil
	}
	switch family {
	case FamilyV4:
		if !src.Is4() && !src.Is4In6() {
			return fmt.Errorf("netbind: source_ip %s is IPv6, expected IPv4", src)
		}
	case FamilyV6:
		if src.Is4() {
			return fmt.Errorf("netbind: source_ip %s is IPv4, expected IPv6", src)
		}
	}
	return nil
}
