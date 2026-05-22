// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf6

type DatabaseOpts struct {
	Detail         bool
	Dump           bool
	Internal       bool
	SelfOriginated bool
	AdvRouter      string
	LinkStateID    string
}
