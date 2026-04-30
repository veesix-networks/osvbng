// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package subscriber

import (
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

var sessionLifecycle = telemetry.MustRegisterCounter(telemetry.CounterOpts{
	Name:   "subscriber.sessions.lifecycle",
	Help:   "Session lifecycle transitions, partitioned by access type, protocol, and terminal state.",
	Labels: []string{"access_type", "protocol", "event"},
})

func (c *Component) emitLifecycleCounter(s models.SubscriberSession) {
	var event string
	switch s.GetState() {
	case models.SessionStateActive:
		event = "established"
	case models.SessionStateReleased:
		event = "released"
	default:
		return
	}
	sessionLifecycle.WithLabelValues(string(s.GetAccessType()), string(s.GetProtocol()), event).Inc()
}
