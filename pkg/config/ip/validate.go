// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ip

import (
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/netbind"
)

func (o *IPv4DHCPOptions) Validate(family netbind.Family, lookup netbind.VRFLookup) error {
	if err := o.EndpointBinding.Validate(family, lookup); err != nil {
		return fmt.Errorf("relay-agent binding: %w", err)
	}
	for i, srv := range o.Servers {
		effective := srv.EndpointBinding.MergeWith(o.EndpointBinding)
		if err := effective.Validate(family, lookup); err != nil {
			return fmt.Errorf("servers[%d] %s: %w", i, srv.Address, err)
		}
	}
	return nil
}

func (o *IPv6DHCPv6Options) Validate(family netbind.Family, lookup netbind.VRFLookup) error {
	if err := o.EndpointBinding.Validate(family, lookup); err != nil {
		return fmt.Errorf("relay-agent binding: %w", err)
	}
	for i, srv := range o.Servers {
		effective := srv.EndpointBinding.MergeWith(o.EndpointBinding)
		if err := effective.Validate(family, lookup); err != nil {
			return fmt.Errorf("servers[%d] %s: %w", i, srv.Address, err)
		}
	}
	return nil
}
