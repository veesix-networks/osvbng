// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package vpp

import "errors"

// retvalEntryNeedsRefresh matches VNET_API_ERROR_ENTRY_NEEDS_REFRESH defined
// (with the same numeric value) in osvbng-vpp-plugin-ipoe, osvbng-vpp-plugin-
// pppoe-control, and osvbng-vpp-plugin-cgnat. Returned by the plugins'
// add-session / add-mapping APIs when an entry already exists for the lookup
// key but one or more mutable inputs have drifted from the stored record;
// the plugin still populates the outgoing sw_if_index / mapping handle so the
// caller can delete-and-recreate to converge.
const retvalEntryNeedsRefresh = -500

// ErrEntryNeedsRefresh is the sentinel surfaced through callers that want to
// observe the plugin's three-state idempotency contract directly. The
// southbound's Add* helpers already perform the delete-and-recreate inline so
// most code never sees it; it exists for tests and for diagnostic paths that
// want to log the refresh distinctly from a fresh-create.
var ErrEntryNeedsRefresh = errors.New("plugin entry exists with drifted mutable inputs; refresh required")
