// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package vrfmgr

import (
	"sync/atomic"

	"github.com/veesix-networks/osvbng/pkg/models/vrf"
)

type Resolver interface {
	ResolveVRF(name string) (tableID uint32, ipv4 bool, ipv6 bool, err error)
	GetVRFs() []vrf.VRF
}

var _ Resolver = (*Manager)(nil)

var current atomic.Pointer[Manager]

func Set(m *Manager) {
	current.Store(m)
}

// Get returns nil until the routing component has called Set.
func Get() Resolver {
	m := current.Load()
	if m == nil {
		return nil
	}
	return m
}
