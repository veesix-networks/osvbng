// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
)

func ValidateSubscriberGroupVRF(cfg *Config) error {
	if cfg == nil || cfg.SubscriberGroups == nil {
		return nil
	}

	for name, group := range cfg.SubscriberGroups.Groups {
		if group == nil {
			continue
		}

		for _, vlanRange := range group.VLANs {
			if vlanRange.Interface == "" {
				continue
			}

			resolvedVRF := subscriberRangeVRF(group, vlanRange)
			if resolvedVRF != "" && cfg.VRFS[resolvedVRF] == nil {
				return fmt.Errorf("subscriber group %q vlan %q: vrf %q is not declared under vrfs: section", name, vlanRange.SVLAN, resolvedVRF)
			}

			loopback, ok := cfg.Interfaces[vlanRange.Interface]
			if !ok || loopback == nil {
				return fmt.Errorf("subscriber group %q vlan %q: interface %q is not declared under interfaces: section", name, vlanRange.SVLAN, vlanRange.Interface)
			}

			loopbackVRF := loopback.VRF
			if loopbackVRF != "" && resolvedVRF == "" {
				return fmt.Errorf("subscriber group %q vlan %q: interface %q is in VRF %q, but neither range nor group declares vrf (use vrf: \"default\" to explicitly opt out)", name, vlanRange.SVLAN, vlanRange.Interface, loopbackVRF)
			}
			if resolvedVRF != "" && resolvedVRF != loopbackVRF {
				return fmt.Errorf("subscriber group %q vlan %q: resolved vrf %q does not match interface %q vrf %q", name, vlanRange.SVLAN, resolvedVRF, vlanRange.Interface, loopbackVRF)
			}
		}
	}

	return nil
}

func subscriberRangeVRF(group *subscriber.SubscriberGroup, r subscriber.VLANRange) string {
	if r.VRF == "default" {
		return ""
	}
	if r.VRF != "" {
		return r.VRF
	}
	if group.VRF == "default" {
		return ""
	}
	return group.VRF
}
