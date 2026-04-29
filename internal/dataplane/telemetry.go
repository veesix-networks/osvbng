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

type dataplaneTelemetry struct {
	systemVectorRate     *telemetry.GaugeHandle
	systemInputRate      *telemetry.GaugeHandle
	systemLastUpdate     *telemetry.GaugeHandle
	systemLastStatsClear *telemetry.GaugeHandle
	systemHeartbeat      *telemetry.CounterHandle
	systemWorkerThreads  *telemetry.GaugeHandle

	memoryTotal      *telemetry.Gauge
	memoryUsed       *telemetry.Gauge
	memoryFree       *telemetry.Gauge
	memoryUsedMMap   *telemetry.Gauge
	memoryTotalAlloc *telemetry.Counter
	memoryFreeChunks *telemetry.Gauge
	memoryReleasable *telemetry.Gauge

	ifaceRxPackets *telemetry.Counter
	ifaceRxBytes   *telemetry.Counter
	ifaceRxErrors  *telemetry.Counter
	ifaceTxPackets *telemetry.Counter
	ifaceTxBytes   *telemetry.Counter
	ifaceTxErrors  *telemetry.Counter
	ifaceDrops     *telemetry.Counter
	ifacePunts     *telemetry.Counter
	ifaceAdminUp   *telemetry.Gauge
	ifaceLinkUp    *telemetry.Gauge

	nodeCalls    *telemetry.Counter
	nodeVectors  *telemetry.Counter
	nodeSuspends *telemetry.Counter
	nodeClocks   *telemetry.Counter

	errorCount *telemetry.Counter

	bufferCached    *telemetry.Gauge
	bufferUsed      *telemetry.Gauge
	bufferAvailable *telemetry.Gauge
}

func newDataplaneTelemetry(reg *telemetry.Registry) (*dataplaneTelemetry, error) {
	t := &dataplaneTelemetry{}

	gauge := func(opts telemetry.GaugeOpts) *telemetry.Gauge {
		g, err := reg.RegisterGauge(opts)
		if err != nil {
			panic(err)
		}
		return g
	}
	counter := func(opts telemetry.CounterOpts) *telemetry.Counter {
		c, err := reg.RegisterCounter(opts)
		if err != nil {
			panic(err)
		}
		return c
	}

	sysVecRate := gauge(telemetry.GaugeOpts{Name: "vpp_system_vector_rate", Help: "VPP vector rate."})
	sysInputRate := gauge(telemetry.GaugeOpts{Name: "vpp_system_input_rate", Help: "VPP input rate."})
	sysLastUpdate := gauge(telemetry.GaugeOpts{Name: "vpp_system_last_update", Help: "VPP last stats update timestamp."})
	sysLastStatsClear := gauge(telemetry.GaugeOpts{Name: "vpp_system_last_stats_clear", Help: "VPP last stats clear timestamp."})
	sysHeartbeat := counter(telemetry.CounterOpts{Name: "vpp_system_heartbeat", Help: "VPP heartbeat counter."})
	sysWorkers := gauge(telemetry.GaugeOpts{Name: "vpp_system_worker_threads", Help: "VPP worker thread count."})

	t.systemVectorRate = sysVecRate.WithLabelValues()
	t.systemInputRate = sysInputRate.WithLabelValues()
	t.systemLastUpdate = sysLastUpdate.WithLabelValues()
	t.systemLastStatsClear = sysLastStatsClear.WithLabelValues()
	t.systemHeartbeat = sysHeartbeat.WithLabelValues()
	t.systemWorkerThreads = sysWorkers.WithLabelValues()

	t.memoryTotal = gauge(telemetry.GaugeOpts{Name: "vpp_memory_total_bytes", Help: "VPP heap total bytes.", Labels: []string{"heap"}})
	t.memoryUsed = gauge(telemetry.GaugeOpts{Name: "vpp_memory_used_bytes", Help: "VPP heap used bytes.", Labels: []string{"heap"}})
	t.memoryFree = gauge(telemetry.GaugeOpts{Name: "vpp_memory_free_bytes", Help: "VPP heap free bytes.", Labels: []string{"heap"}})
	t.memoryUsedMMap = gauge(telemetry.GaugeOpts{Name: "vpp_memory_used_mmap_bytes", Help: "VPP heap used mmap bytes.", Labels: []string{"heap"}})
	t.memoryTotalAlloc = counter(telemetry.CounterOpts{Name: "vpp_memory_total_alloc_bytes", Help: "VPP heap total allocated bytes.", Labels: []string{"heap"}})
	t.memoryFreeChunks = gauge(telemetry.GaugeOpts{Name: "vpp_memory_free_chunks", Help: "VPP heap free chunks.", Labels: []string{"heap"}})
	t.memoryReleasable = gauge(telemetry.GaugeOpts{Name: "vpp_memory_releasable_bytes", Help: "VPP heap releasable bytes.", Labels: []string{"heap"}})

	ifLabels := []string{"name", "index"}
	t.ifaceRxPackets = counter(telemetry.CounterOpts{Name: "vpp_interface_rx_packets", Help: "VPP per-interface received packets.", Labels: ifLabels})
	t.ifaceRxBytes = counter(telemetry.CounterOpts{Name: "vpp_interface_rx_bytes", Help: "VPP per-interface received bytes.", Labels: ifLabels})
	t.ifaceRxErrors = counter(telemetry.CounterOpts{Name: "vpp_interface_rx_errors", Help: "VPP per-interface receive errors.", Labels: ifLabels})
	t.ifaceTxPackets = counter(telemetry.CounterOpts{Name: "vpp_interface_tx_packets", Help: "VPP per-interface transmitted packets.", Labels: ifLabels})
	t.ifaceTxBytes = counter(telemetry.CounterOpts{Name: "vpp_interface_tx_bytes", Help: "VPP per-interface transmitted bytes.", Labels: ifLabels})
	t.ifaceTxErrors = counter(telemetry.CounterOpts{Name: "vpp_interface_tx_errors", Help: "VPP per-interface transmit errors.", Labels: ifLabels})
	t.ifaceDrops = counter(telemetry.CounterOpts{Name: "vpp_interface_drops", Help: "VPP per-interface dropped packets.", Labels: ifLabels})
	t.ifacePunts = counter(telemetry.CounterOpts{Name: "vpp_interface_punts", Help: "VPP per-interface punted packets.", Labels: ifLabels})
	t.ifaceAdminUp = gauge(telemetry.GaugeOpts{Name: "vpp_interface_admin_up", Help: "1 if interface admin state is up.", Labels: ifLabels})
	t.ifaceLinkUp = gauge(telemetry.GaugeOpts{Name: "vpp_interface_link_up", Help: "1 if interface link state is up.", Labels: ifLabels})

	nodeLabels := []string{"name", "index"}
	t.nodeCalls = counter(telemetry.CounterOpts{Name: "vpp_node_calls", Help: "VPP graph node calls.", Labels: nodeLabels})
	t.nodeVectors = counter(telemetry.CounterOpts{Name: "vpp_node_vectors", Help: "VPP graph node vectors processed.", Labels: nodeLabels})
	t.nodeSuspends = counter(telemetry.CounterOpts{Name: "vpp_node_suspends", Help: "VPP graph node suspends.", Labels: nodeLabels})
	t.nodeClocks = counter(telemetry.CounterOpts{Name: "vpp_node_clocks", Help: "VPP graph node clock cycles.", Labels: nodeLabels})

	t.errorCount = counter(telemetry.CounterOpts{Name: "vpp_error_count", Help: "VPP error counter.", Labels: []string{"name"}})

	bufLabels := []string{"pool_name"}
	t.bufferCached = gauge(telemetry.GaugeOpts{Name: "vpp_buffer_cached", Help: "VPP cached buffers.", Labels: bufLabels})
	t.bufferUsed = gauge(telemetry.GaugeOpts{Name: "vpp_buffer_used", Help: "VPP used buffers.", Labels: bufLabels})
	t.bufferAvailable = gauge(telemetry.GaugeOpts{Name: "vpp_buffer_available", Help: "VPP available buffers.", Labels: bufLabels})

	return t, nil
}

func (c *Component) startTelemetryLoop() {
	c.readLoopWg.Add(1)
	go c.telemetryStatsLoop()
}

func (c *Component) telemetryStatsLoop() {
	defer c.readLoopWg.Done()

	if err := c.pollVPPStats(); err != nil {
		c.logger.Debug("Initial VPP telemetry poll failed", "error", err)
	}

	ticker := time.NewTicker(dataplanePollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.readCtx.Done():
			return
		case <-ticker.C:
			if err := c.pollVPPStats(); err != nil {
				c.logger.Debug("VPP telemetry poll failed", "error", err)
			}
		}
	}
}

func (c *Component) pollVPPStats() error {
	t := c.telemetry
	if t == nil {
		return nil
	}

	if sys, err := c.vpp.GetSystemStats(); err == nil && sys != nil {
		t.systemVectorRate.Set(float64(sys.VectorRate))
		t.systemInputRate.Set(float64(sys.InputRate))
		t.systemLastUpdate.Set(float64(sys.LastUpdate))
		t.systemLastStatsClear.Set(float64(sys.LastStatsClear))
		bumpCounterAbs(t.systemHeartbeat, sys.Heartbeat)
		t.systemWorkerThreads.Set(float64(sys.NumWorkerThreads))
	}

	if mem, err := c.vpp.GetMemoryStats(); err == nil {
		for _, m := range mem {
			t.memoryTotal.WithLabelValues(m.Heap).Set(float64(m.Total))
			t.memoryUsed.WithLabelValues(m.Heap).Set(float64(m.Used))
			t.memoryFree.WithLabelValues(m.Heap).Set(float64(m.Free))
			t.memoryUsedMMap.WithLabelValues(m.Heap).Set(float64(m.UsedMMap))
			bumpCounterAbs(t.memoryTotalAlloc.WithLabelValues(m.Heap), m.TotalAlloc)
			t.memoryFreeChunks.WithLabelValues(m.Heap).Set(float64(m.FreeChunks))
			t.memoryReleasable.WithLabelValues(m.Heap).Set(float64(m.Releasable))
		}
	}

	if ifs, err := c.vpp.GetInterfaceStats(); err == nil {
		for _, ii := range ifs {
			emitInterfaceCounters(t, ii)
		}
	}

	if nodes, err := c.vpp.GetNodeStats(); err == nil {
		for _, n := range nodes {
			idx := strconv.FormatUint(uint64(n.Index), 10)
			bumpCounterAbs(t.nodeCalls.WithLabelValues(n.Name, idx), n.Calls)
			bumpCounterAbs(t.nodeVectors.WithLabelValues(n.Name, idx), n.Vectors)
			bumpCounterAbs(t.nodeSuspends.WithLabelValues(n.Name, idx), n.Suspends)
			bumpCounterAbs(t.nodeClocks.WithLabelValues(n.Name, idx), n.Clocks)
		}
	}

	if errs, err := c.vpp.GetErrorStats(); err == nil {
		for _, e := range errs {
			bumpCounterAbs(t.errorCount.WithLabelValues(e.Name), e.Count)
		}
	}

	if bufs, err := c.vpp.GetBufferStats(); err == nil {
		for _, b := range bufs {
			t.bufferCached.WithLabelValues(b.PoolName).Set(b.Cached)
			t.bufferUsed.WithLabelValues(b.PoolName).Set(b.Used)
			t.bufferAvailable.WithLabelValues(b.PoolName).Set(b.Available)
		}
	}

	return nil
}

func emitInterfaceCounters(t *dataplaneTelemetry, ii southbound.InterfaceStats) {
	idx := strconv.FormatUint(uint64(ii.Index), 10)
	bumpCounterAbs(t.ifaceRxPackets.WithLabelValues(ii.Name, idx), ii.Rx)
	bumpCounterAbs(t.ifaceRxBytes.WithLabelValues(ii.Name, idx), ii.RxBytes)
	bumpCounterAbs(t.ifaceRxErrors.WithLabelValues(ii.Name, idx), ii.RxErrors)
	bumpCounterAbs(t.ifaceTxPackets.WithLabelValues(ii.Name, idx), ii.Tx)
	bumpCounterAbs(t.ifaceTxBytes.WithLabelValues(ii.Name, idx), ii.TxBytes)
	bumpCounterAbs(t.ifaceTxErrors.WithLabelValues(ii.Name, idx), ii.TxErrors)
	bumpCounterAbs(t.ifaceDrops.WithLabelValues(ii.Name, idx), ii.Drops)
	bumpCounterAbs(t.ifacePunts.WithLabelValues(ii.Name, idx), ii.Punts)
}

// bumpCounterAbs maps a monotonic absolute value (VPP stats segment) onto the
// SDK counter handle. The SDK counter only accepts increments, so the handle
// stores the cumulative value across polls. On VPP restart or stats clear the
// absolute reading goes backwards; we re-baseline by treating the new value
// as a fresh increment from zero.
func bumpCounterAbs(h *telemetry.CounterHandle, current uint64) {
	if h == nil {
		return
	}
	last := h.Value()
	if current >= last {
		h.Add(current - last)
		return
	}
	h.Add(current)
}

func (c *Component) handleInterfaceState(event events.Event) {
	if c.telemetry == nil {
		return
	}
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
	c.telemetry.ifaceAdminUp.WithLabelValues(name, idx).Set(adminVal)
	c.telemetry.ifaceLinkUp.WithLabelValues(name, idx).Set(linkVal)

	if data.Deleted {
		c.telemetry.ifaceAdminUp.UnregisterSeries(name, idx)
		c.telemetry.ifaceLinkUp.UnregisterSeries(name, idx)
	}
}
