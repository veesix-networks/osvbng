// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package aaa

import "github.com/veesix-networks/osvbng/pkg/telemetry"

// UsernameEmptyDrops counts AAA-publish attempts dropped because a configured
// policy.format expanded to an empty User-Name. Lives in pkg/aaa (a leaf both
// the IPoE and PPPoE packages already import) so the single counter can be
// shared across access types without an import cycle.
var UsernameEmptyDrops = telemetry.MustRegisterCounter(telemetry.CounterOpts{
	Name:   "aaa.policy.username_empty_drops",
	Help:   "AAA-publish attempts dropped because the configured policy.format expanded to an empty User-Name. Labels: policy name, subscriber-group name, access type.",
	Labels: []string{"policy", "group", "access_type"},
})
