// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package dataplane

import (
	"strconv"

	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

var (
	ifaceAdminUp = telemetry.MustRegisterGauge(telemetry.GaugeOpts{
		Name:   "dataplane.vpp.interface.admin_up",
		Help:   "1 if interface admin state is up.",
		Labels: []string{"name", "index"},
	})
	ifaceLinkUp = telemetry.MustRegisterGauge(telemetry.GaugeOpts{
		Name:   "dataplane.vpp.interface.link_up",
		Help:   "1 if interface link state is up.",
		Labels: []string{"name", "index"},
	})
)

func (c *Component) handleInterfaceState(event events.Event) {
	var data events.InterfaceStateEvent
	switch d := event.Data.(type) {
	case events.InterfaceStateEvent:
		data = d
	case *events.InterfaceStateEvent:
		if d == nil {
			return
		}
		data = *d
	default:
		return
	}

	idx := strconv.FormatUint(uint64(data.SwIfIndex), 10)
	name := data.Name
	if name == "" {
		name = "sw_if_index_" + idx
	}

	adminVal := 0.0
	if data.AdminUp {
		adminVal = 1
	}
	linkVal := 0.0
	if data.LinkUp {
		linkVal = 1
	}
	ifaceAdminUp.WithLabelValues(name, idx).Set(adminVal)
	ifaceLinkUp.WithLabelValues(name, idx).Set(linkVal)

	if data.Deleted {
		ifaceAdminUp.UnregisterSeries(name, idx)
		ifaceLinkUp.UnregisterSeries(name, idx)
	}
}
