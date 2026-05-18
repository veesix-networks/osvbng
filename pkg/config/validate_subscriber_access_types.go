// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"fmt"
	"sort"

	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
)

var validAccessTypes = map[subscriber.AccessType]struct{}{
	subscriber.AccessTypeIPoE:  {},
	subscriber.AccessTypePPPoE: {},
	subscriber.AccessTypeLAC:   {},
	subscriber.AccessTypeLNS:   {},
}

func ValidateSubscriberAccessTypes(cfg *Config) error {
	if cfg == nil || cfg.SubscriberGroups == nil {
		return nil
	}

	parentInterfaces := map[string]struct{}{}

	for name, group := range cfg.SubscriberGroups.Groups {
		if group == nil {
			continue
		}

		if len(group.AccessTypes) > 0 {
			if len(group.AccessTypes) != 1 || group.AccessTypes[0] != subscriber.AccessTypeLNS {
				return fmt.Errorf("subscriber group %q: group-level access-types is only valid for LNS-only groups (got %v); other protocols must declare access-types per vlan range", name, group.AccessTypes)
			}
			if len(group.VLANs) > 0 {
				return fmt.Errorf("subscriber group %q: LNS-only groups must not declare vlans (L2TP-terminated subscribers do not arrive via SVLAN demux)", name)
			}
			continue
		}

		if len(group.VLANs) == 0 {
			return fmt.Errorf("subscriber group %q: vlans is empty", name)
		}

		for i, vr := range group.VLANs {
			if err := validateVLANRangeAccessTypes(name, i, &vr); err != nil {
				return err
			}

			needsParent := !vr.HasAccessType(subscriber.AccessTypeLNS)
			if needsParent {
				if vr.ParentInterface == "" {
					return fmt.Errorf("subscriber group %q vlans[%d]: parent-interface is required when access-types is %v", name, i, vr.AccessTypes)
				}
				if _, ok := cfg.Interfaces[vr.ParentInterface]; !ok {
					return fmt.Errorf("subscriber group %q vlans[%d]: parent-interface %q is not defined in interfaces", name, i, vr.ParentInterface)
				}
				parentInterfaces[vr.ParentInterface] = struct{}{}
			} else if vr.ParentInterface != "" {
				if _, ok := cfg.Interfaces[vr.ParentInterface]; !ok {
					return fmt.Errorf("subscriber group %q vlans[%d]: parent-interface %q is not defined in interfaces", name, i, vr.ParentInterface)
				}
			}
		}
	}

	if len(parentInterfaces) > 1 {
		names := make([]string, 0, len(parentInterfaces))
		for n := range parentInterfaces {
			names = append(names, n)
		}
		sort.Strings(names)
		return fmt.Errorf("subscriber-groups reference multiple parent-interfaces %v: only one access interface is supported", names)
	}

	return nil
}

func validateVLANRangeAccessTypes(groupName string, idx int, vr *subscriber.VLANRange) error {
	if len(vr.AccessTypes) == 0 {
		return fmt.Errorf("subscriber group %q vlans[%d]: access-types is empty", groupName, idx)
	}

	seen := map[subscriber.AccessType]struct{}{}
	for _, a := range vr.AccessTypes {
		if _, ok := validAccessTypes[a]; !ok {
			return fmt.Errorf("subscriber group %q vlans[%d]: access-types entry %q is not one of ipoe, pppoe, lac, lns", groupName, idx, a)
		}
		if _, dup := seen[a]; dup {
			return fmt.Errorf("subscriber group %q vlans[%d]: access-types contains duplicate %q", groupName, idx, a)
		}
		seen[a] = struct{}{}
	}

	hasIPoE := vr.HasAccessType(subscriber.AccessTypeIPoE)
	hasPPPoE := vr.HasAccessType(subscriber.AccessTypePPPoE)
	hasLAC := vr.HasAccessType(subscriber.AccessTypeLAC)
	hasLNS := vr.HasAccessType(subscriber.AccessTypeLNS)

	if hasLAC && (hasLNS || hasIPoE || hasPPPoE) {
		return fmt.Errorf("subscriber group %q vlans[%d]: lac is mutually exclusive with other access-types (got %v)", groupName, idx, vr.AccessTypes)
	}
	if hasLNS && (hasLAC || hasIPoE || hasPPPoE) {
		return fmt.Errorf("subscriber group %q vlans[%d]: lns is mutually exclusive with other access-types (got %v)", groupName, idx, vr.AccessTypes)
	}
	if len(vr.AccessTypes) > 1 && (!hasIPoE || !hasPPPoE || hasLAC || hasLNS) {
		return fmt.Errorf("subscriber group %q vlans[%d]: the only valid multi-element access-types combination is [ipoe, pppoe] (got %v)", groupName, idx, vr.AccessTypes)
	}
	return nil
}
