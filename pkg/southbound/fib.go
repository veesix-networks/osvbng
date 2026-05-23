// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package southbound

import "net/netip"

type FIB interface {
	GetIPv4FIB(tableID uint32) ([]*IPFIBEntry, error)
	GetIPv6FIB(tableID uint32) ([]*IPFIBEntry, error)
	GetIPv4FIBAll() (map[uint32][]*IPFIBEntry, error)
	GetIPv6FIBAll() (map[uint32][]*IPFIBEntry, error)
	LookupIPv4FIB(tableID uint32, prefix netip.Prefix) (*IPFIBEntry, error)
	LookupIPv6FIB(tableID uint32, prefix netip.Prefix) (*IPFIBEntry, error)
	GetIPv4FIBSummary() (*IPFIBSummaryAll, error)
	GetIPv6FIBSummary() (*IPFIBSummaryAll, error)
}
