// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package subscriber

// AccessResolver answers runtime queries about subscriber access-type
// composition. Implementations are responsible for keeping their internal
// state in sync with the running config (including operator-driven conf
// commands that change access-types on a VLAN range at runtime).
type AccessResolver interface {
	// IsMixedAccessSVLAN reports whether the VLAN range matching svlan
	// has both ipoe and pppoe in its access-types. Used by session-bind
	// fast paths to decide whether to engage the cross-protocol
	// exclusivity mediator.
	IsMixedAccessSVLAN(svlan uint16) bool
}
