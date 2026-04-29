// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package dataplane

import (
	"strconv"
	"time"

	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

const dataplanePollInterval = 10 * time.Second

var (
	systemMetrics    = telemetry.MustRegisterStruct[southbound.SystemStats](telemetry.RegisterOpts{Path: "dataplane.vpp"})
	memoryMetrics    = telemetry.MustRegisterStruct[southbound.MemoryStats](telemetry.RegisterOpts{Path: "dataplane.vpp"})
	interfaceMetrics = telemetry.MustRegisterStruct[southbound.InterfaceStats](telemetry.RegisterOpts{Path: "dataplane.vpp"})
	nodeMetrics      = telemetry.MustRegisterStruct[southbound.NodeStats](telemetry.RegisterOpts{Path: "dataplane.vpp"})
	errorMetrics     = telemetry.MustRegisterStruct[southbound.ErrorStats](telemetry.RegisterOpts{Path: "dataplane.vpp"})
	bufferMetrics    = telemetry.MustRegisterStruct[southbound.BufferStats](telemetry.RegisterOpts{Path: "dataplane.vpp"})

	systemHandle = systemMetrics.WithLabelValues()
	ifaceAdminUp = telemetry.MustRegisterGauge(telemetry.GaugeOpts{Name: "dataplane.vpp.interface.admin_up", Help: "1 if interface admin state is up.", Labels: []string{"name", "index"}})
	ifaceLinkUp  = telemetry.MustRegisterGauge(telemetry.GaugeOpts{Name: "dataplane.vpp.interface.link_up", Help: "1 if interface link state is up.", Labels: []string{"name", "index"}})
)

func (c *Component) startTelemetryLoop() {
	c.readLoopWg.Add(1)
	go c.telemetryStatsLoop()
}

func (c *Component) telemetryStatsLoop() {
	defer c.readLoopWg.Done()

	c.pollVPPStats()

	ticker := time.NewTicker(dataplanePollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.readCtx.Done():
			return
		case <-ticker.C:
			c.pollVPPStats()
		}
	}
}

func (c *Component) pollVPPStats() {
	if sys, err := c.vpp.GetSystemStats(); err == nil && sys != nil {
		emit(systemMetrics, systemHandle, sys)
	}
	if mem, err := c.vpp.GetMemoryStats(); err == nil {
		for i := range mem {
			emit(memoryMetrics, memoryMetrics.WithLabelValues(mem[i].Heap), &mem[i])
		}
	}
	if ifs, err := c.vpp.GetInterfaceStats(); err == nil {
		for i := range ifs {
			emit(interfaceMetrics, interfaceMetrics.WithLabelValues(ifs[i].Name, strconv.FormatUint(uint64(ifs[i].Index), 10)), &ifs[i])
		}
	}
	if nodes, err := c.vpp.GetNodeStats(); err == nil {
		for i := range nodes {
			emit(nodeMetrics, nodeMetrics.WithLabelValues(nodes[i].Name, strconv.FormatUint(uint64(nodes[i].Index), 10)), &nodes[i])
		}
	}
	if errs, err := c.vpp.GetErrorStats(); err == nil {
		for i := range errs {
			emit(errorMetrics, errorMetrics.WithLabelValues(errs[i].Name), &errs[i])
		}
	}
	if bufs, err := c.vpp.GetBufferStats(); err == nil {
		for i := range bufs {
			emit(bufferMetrics, bufferMetrics.WithLabelValues(bufs[i].PoolName), &bufs[i])
		}
	}
}

// emit copies counter/gauge field values from src into the registered SDK
// metrics for type T. Counters take the absolute-to-delta transformation
// since VPP's stats segment exposes monotonic absolute values.
func emit[T any](m *telemetry.StructMetrics[T], h *telemetry.StructHandles, src *T) {
	m.EmitFrom(h, src)
}

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
