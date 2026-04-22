// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
)

const (
	defaultEthernetMTU uint16 = 1500
	pppoePPPHeader     uint16 = 8
	dot1qOverhead      uint16 = 4
	qinqOverhead       uint16 = 8
)

func ValidateMSSClampParentMTU(cfg *Config) error {
	if cfg == nil || cfg.SubscriberGroups == nil {
		return nil
	}

	for name, group := range cfg.SubscriberGroups.Groups {
		if group == nil {
			continue
		}

		if group.PPPoE != nil && group.PPPoE.IsBabyGiants() {
			if group.AccessType != "pppoe" {
				return fmt.Errorf("subscriber group %q: pppoe.mru only applies to PPPoE access (current access-type: %q)", name, group.AccessType)
			}
		}

		if !pppMRUValidationNeeded(group) {
			continue
		}

		access, err := cfg.GetAccessInterface()
		if err != nil {
			return fmt.Errorf("subscriber group %q: %w", name, err)
		}

		parent, ok := cfg.Interfaces[access]
		if !ok || parent == nil {
			return fmt.Errorf("subscriber group %q: access interface %q not found in config", name, access)
		}

		parentMTU := uint16(parent.MTU)
		if parent.MTU == 0 {
			parentMTU = defaultEthernetMTU
		}

		required := requiredParentMTU(group)
		if parentMTU < required {
			return fmt.Errorf(
				"subscriber group %q: pppoe.mru=%d requires parent interface %q mtu >= %d (current: %d). Configure the parent interface MTU before raising pppoe.mru, or set pppoe.mru <= 1492",
				name, group.PPPoE.GetMRU(), access, required, parentMTU,
			)
		}
	}

	return nil
}

func pppMRUValidationNeeded(group *subscriber.SubscriberGroup) bool {
	return group.AccessType == "pppoe" && group.PPPoE != nil && group.PPPoE.IsBabyGiants()
}

func requiredParentMTU(group *subscriber.SubscriberGroup) uint16 {
	mru := subscriber.DefaultPPPMRU
	if group.AccessType == "pppoe" && group.PPPoE != nil {
		mru = group.PPPoE.GetMRU()
	}

	overhead := uint16(0)
	if group.AccessType == "pppoe" {
		overhead += pppoePPPHeader
	}
	overhead += vlanOverheadFor(group)

	return mru + overhead
}

func vlanOverheadFor(group *subscriber.SubscriberGroup) uint16 {
	for _, vr := range group.VLANs {
		if vr.CVLAN == "" || vr.CVLAN == "any" {
			continue
		}
		return qinqOverhead
	}
	return dot1qOverhead
}
