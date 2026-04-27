// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package netbind

import (
	"errors"
	"fmt"
	"net/netip"

	"github.com/vishvananda/netlink"
)

// VRFResolver is the subset of vrfmgr.Manager that ValidateBinding needs.
// Defined as an interface to avoid an import dependency on pkg/vrfmgr.
type VRFResolver interface {
	ResolveVRF(name string) (uint32, bool, bool, error)
}

// LinkLister wraps the subset of *netlink.Handle ValidateBinding uses.
type LinkLister interface {
	LinkList() ([]netlink.Link, error)
	LinkByName(name string) (netlink.Link, error)
	AddrList(link netlink.Link, family int) ([]netlink.Addr, error)
}

func ValidateBinding(vrfMgr VRFResolver, nl LinkLister, b Binding, family Family) error {
	if b.IsZero() {
		return nil
	}

	if family != FamilyV4 && family != FamilyV6 {
		return fmt.Errorf("netbind: invalid family %d", family)
	}

	if err := validateSourceIPFamily(b.SourceIP, family); err != nil {
		return err
	}

	if b.VRF != "" {
		if err := validateVRFExists(vrfMgr, b.VRF, family); err != nil {
			return err
		}
	}

	if b.SourceIP.IsValid() && nl != nil {
		if err := validateSourceIPOnInterface(nl, b.SourceIP, b.VRF); err != nil {
			return err
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

func validateVRFExists(vrfMgr VRFResolver, vrf string, family Family) error {
	if vrfMgr == nil {
		return errors.New("netbind: VRF resolver not available")
	}
	_, hasV4, hasV6, err := vrfMgr.ResolveVRF(vrf)
	if err != nil {
		return fmt.Errorf("netbind: vrf %q: %w", vrf, err)
	}
	switch family {
	case FamilyV4:
		if !hasV4 {
			return fmt.Errorf("netbind: vrf %q does not have ipv4-unicast enabled", vrf)
		}
	case FamilyV6:
		if !hasV6 {
			return fmt.Errorf("netbind: vrf %q does not have ipv6-unicast enabled", vrf)
		}
	}
	return nil
}

func validateSourceIPOnInterface(nl LinkLister, src netip.Addr, vrf string) error {
	links, err := nl.LinkList()
	if err != nil {
		return fmt.Errorf("netbind: list links: %w", err)
	}

	var vrfLink netlink.Link
	if vrf != "" {
		vrfLink, err = nl.LinkByName(vrf)
		if err != nil {
			return fmt.Errorf("netbind: vrf master %q not found: %w", vrf, err)
		}
	}

	wantFamily := netlink.FAMILY_V4
	if !src.Is4() && !src.Is4In6() {
		wantFamily = netlink.FAMILY_V6
	}

	srcStd := src.As16()
	srcSlice := srcStd[:]
	if src.Is4() {
		v4 := src.As4()
		srcSlice = v4[:]
	}

	for _, link := range links {
		addrs, err := nl.AddrList(link, wantFamily)
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if a.IPNet == nil || a.IPNet.IP == nil {
				continue
			}
			matchSlice := a.IPNet.IP
			if src.Is4() {
				v4 := matchSlice.To4()
				if v4 == nil {
					continue
				}
				matchSlice = v4
			}
			if !ipBytesEqual(matchSlice, srcSlice) {
				continue
			}
			if vrf == "" {
				return nil
			}
			if link.Attrs().MasterIndex == vrfLink.Attrs().Index {
				return nil
			}
			return fmt.Errorf("netbind: source_ip %s is on %q, not enslaved to vrf %q",
				src, link.Attrs().Name, vrf)
		}
	}

	if vrf == "" {
		return fmt.Errorf("netbind: source_ip %s not assigned to any interface", src)
	}
	return fmt.Errorf("netbind: source_ip %s not found on any interface enslaved to vrf %q", src, vrf)
}

func ipBytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
