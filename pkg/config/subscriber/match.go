// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package subscriber

import (
	"fmt"
	"sort"
)

// GroupMatch's Group and VR point into the config generation the index was
// built from, so one lookup classifies a subscriber without a separate
// running-config read.
type GroupMatch struct {
	Name  string
	Group *SubscriberGroup
	VR    *VLANRange
}

type cvlanEntry struct {
	byCVLAN map[uint16]GroupMatch
	any     *GroupMatch
}

type MatchIndex struct {
	bySVLAN map[uint16]*cvlanEntry
}

// BuildMatchIndex walks groups in sorted name order so first-wins resolution is
// deterministic across rebuilds. Unparseable ranges are skipped here;
// ValidateMatchIndex rejects hard collisions before commit.
func BuildMatchIndex(groups *SubscriberGroupsConfig) *MatchIndex {
	idx := &MatchIndex{bySVLAN: make(map[uint16]*cvlanEntry)}
	if groups == nil {
		return idx
	}

	names := make([]string, 0, len(groups.Groups))
	for name, g := range groups.Groups {
		if g != nil {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	for _, name := range names {
		g := groups.Groups[name]
		for i := range g.VLANs {
			vr := &g.VLANs[i]
			svlans, err := vr.GetSVLANs()
			if err != nil {
				continue
			}
			isAny, cvlan, err := vr.GetCVLAN()
			if err != nil {
				continue
			}
			match := GroupMatch{Name: name, Group: g, VR: vr}
			for _, s := range svlans {
				e := idx.bySVLAN[s]
				if e == nil {
					e = &cvlanEntry{byCVLAN: make(map[uint16]GroupMatch)}
					idx.bySVLAN[s] = e
				}
				if isAny {
					if e.any == nil {
						m := match
						e.any = &m
					}
					continue
				}
				if _, exists := e.byCVLAN[cvlan]; !exists {
					e.byCVLAN[cvlan] = match
				}
			}
		}
	}
	return idx
}

// Lookup prefers a C-VLAN-specific entry over a wildcard; cvlan 0 (no inner
// tag) matches only a wildcard.
func (idx *MatchIndex) Lookup(svlan, cvlan uint16) (GroupMatch, bool) {
	if idx == nil {
		return GroupMatch{}, false
	}
	e := idx.bySVLAN[svlan]
	if e == nil {
		return GroupMatch{}, false
	}
	if m, ok := e.byCVLAN[cvlan]; ok {
		return m, true
	}
	if e.any != nil {
		return *e.any, true
	}
	return GroupMatch{}, false
}

// ValidateMatchIndex rejects two ranges claiming the same specific C-VLAN, or
// two wildcards, on one S-VLAN. A specific C-VLAN alongside a wildcard is fine
// (Lookup defines the precedence).
func ValidateMatchIndex(groups *SubscriberGroupsConfig) error {
	if groups == nil {
		return nil
	}

	type claim struct {
		svlan uint16
		cvlan uint16
		any   bool
	}
	owner := make(map[claim]string)

	names := make([]string, 0, len(groups.Groups))
	for name, g := range groups.Groups {
		if g != nil {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	for _, name := range names {
		g := groups.Groups[name]
		for i := range g.VLANs {
			vr := &g.VLANs[i]
			svlans, err := vr.GetSVLANs()
			if err != nil {
				continue
			}
			isAny, cvlan, err := vr.GetCVLAN()
			if err != nil {
				continue
			}
			for _, s := range svlans {
				c := claim{svlan: s, cvlan: cvlan, any: isAny}
				if prev, dup := owner[c]; dup {
					sel := fmt.Sprintf("cvlan %d", cvlan)
					if isAny {
						sel = "cvlan any"
					}
					return fmt.Errorf("subscriber-group VLAN collision on svlan %d %s: claimed by both %q and %q", s, sel, prev, name)
				}
				owner[c] = name
			}
		}
	}
	return nil
}
