// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package aaa

import "github.com/veesix-networks/osvbng/pkg/telemetry"

// UsernameFallbacks counts AAA requests where the configured policy.format
// could not be resolved (a referenced identity token was absent) and the
// User-Name fell back to the MAC. The request is still published; the auth
// provider decides whether to gate. Lives in pkg/aaa (a leaf both the IPoE
// and PPPoE packages already import) so the single counter can be shared
// across access types without an import cycle.
var UsernameFallbacks = telemetry.MustRegisterCounter(telemetry.CounterOpts{
	Name:   "aaa.policy.username_fallbacks",
	Help:   "AAA requests where the configured policy.format could not be resolved and the User-Name fell back to the MAC. Labels: policy name, subscriber-group name, access type.",
	Labels: []string{"policy", "group", "access_type"},
})
