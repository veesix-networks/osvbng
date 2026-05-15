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

		if len(group.AccessTypes) == 0 {
			return fmt.Errorf("subscriber group %q: access-types is empty", name)
		}

		seen := map[subscriber.AccessType]struct{}{}
		for _, a := range group.AccessTypes {
			if _, ok := validAccessTypes[a]; !ok {
				return fmt.Errorf("subscriber group %q: access-types entry %q is not one of ipoe, pppoe, lac, lns", name, a)
			}
			if _, dup := seen[a]; dup {
				return fmt.Errorf("subscriber group %q: access-types contains duplicate %q", name, a)
			}
			seen[a] = struct{}{}
		}

		if err := validateAccessTypeCombination(name, group); err != nil {
			return err
		}

		if !group.HasAccessType(subscriber.AccessTypeLNS) {
			for i, vr := range group.VLANs {
				if vr.ParentInterface == "" {
					return fmt.Errorf("subscriber group %q vlans[%d]: parent-interface is required when access-types is %v", name, i, group.AccessTypes)
				}
				if _, ok := cfg.Interfaces[vr.ParentInterface]; !ok {
					return fmt.Errorf("subscriber group %q vlans[%d]: parent-interface %q is not defined in interfaces", name, i, vr.ParentInterface)
				}
				parentInterfaces[vr.ParentInterface] = struct{}{}
			}
		} else {
			for _, vr := range group.VLANs {
				if vr.ParentInterface != "" {
					if _, ok := cfg.Interfaces[vr.ParentInterface]; !ok {
						return fmt.Errorf("subscriber group %q: parent-interface %q is not defined in interfaces", name, vr.ParentInterface)
					}
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
		return fmt.Errorf("subscriber groups reference multiple parent-interfaces %v: only one access interface is supported", names)
	}

	return nil
}

func validateAccessTypeCombination(name string, group *subscriber.SubscriberGroup) error {
	hasIPoE := group.HasAccessType(subscriber.AccessTypeIPoE)
	hasPPPoE := group.HasAccessType(subscriber.AccessTypePPPoE)
	hasLAC := group.HasAccessType(subscriber.AccessTypeLAC)
	hasLNS := group.HasAccessType(subscriber.AccessTypeLNS)

	if hasLAC && (hasLNS || hasIPoE || hasPPPoE) {
		return fmt.Errorf("subscriber group %q: lac is mutually exclusive with other access-types (got %v)", name, group.AccessTypes)
	}
	if hasLNS && (hasLAC || hasIPoE || hasPPPoE) {
		return fmt.Errorf("subscriber group %q: lns is mutually exclusive with other access-types (got %v)", name, group.AccessTypes)
	}
	if len(group.AccessTypes) > 1 && (!hasIPoE || !hasPPPoE || hasLAC || hasLNS) {
		return fmt.Errorf("subscriber group %q: the only valid multi-element access-types combination is [ipoe, pppoe] (got %v)", name, group.AccessTypes)
	}
	return nil
}
