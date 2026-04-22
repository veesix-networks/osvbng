// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package southbound

type MSSClampPolicy struct {
	Enabled bool
	IPv4MSS uint16
	IPv6MSS uint16
}

type MSSClamp interface {
	EnableMSSClamp(swIfIndex uint32, policy MSSClampPolicy) error
	DisableMSSClamp(swIfIndex uint32) error
}
