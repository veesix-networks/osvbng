// Copyright 2025 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package metrics

import (
	"context"
	"encoding/json"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	Register(paths.HASync.String(), func(logger *logger.Logger) (MetricHandler, error) {
		return newHASyncMetricHandler(logger), nil
	})
}

type haSyncSRG struct {
	SRGName      string  `json:"srg_name"`
	BacklogDepth int     `json:"backlog_depth"`
	SyncLagSecs  float64 `json:"sync_lag_seconds"`
	Creates      uint64  `json:"creates"`
	Updates      uint64  `json:"updates"`
	Deletes      uint64  `json:"deletes"`
	BulkSyncs    uint64  `json:"bulk_syncs"`
}

type haSyncMetricHandler struct {
	logger *logger.Logger
	descs  map[string]*prometheus.Desc
}

func newHASyncMetricHandler(logger *logger.Logger) *haSyncMetricHandler {
	return &haSyncMetricHandler{
		logger: logger,
		descs: map[string]*prometheus.Desc{
			"updates_total": prometheus.NewDesc(
				"osvbng_ha_sync_updates_total",
				"Total number of sync updates",
				[]string{"srg", "action"}, nil),
			"backlog_size": prometheus.NewDesc(
				"osvbng_ha_sync_backlog_size",
				"Current number of entries in the sync backlog",
				[]string{"srg"}, nil),
			"lag_seconds": prometheus.NewDesc(
				"osvbng_ha_sync_lag_seconds",
				"Time since last sync update",
				[]string{"srg"}, nil),
			"bulk_total": prometheus.NewDesc(
				"osvbng_ha_sync_bulk_total",
				"Total number of bulk sync operations",
				[]string{"srg"}, nil),
		},
	}
}

func (h *haSyncMetricHandler) Name() string {
	return paths.HASync.String()
}

func (h *haSyncMetricHandler) Paths() []string {
	return []string{paths.HASync.String()}
}

func (h *haSyncMetricHandler) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range h.descs {
		ch <- desc
	}
}

func (h *haSyncMetricHandler) Collect(ctx context.Context, store cache.Cache, ch chan<- prometheus.Metric) error {
	data, err := store.Get(ctx, "osvbng:state:"+paths.HASync.String())
	if err != nil {
		return nil
	}

	var srgs []haSyncSRG
	if err := json.Unmarshal(data, &srgs); err != nil {
		return err
	}

	for _, s := range srgs {
		ch <- prometheus.MustNewConstMetric(h.descs["updates_total"], prometheus.CounterValue, float64(s.Creates), s.SRGName, "create")
		ch <- prometheus.MustNewConstMetric(h.descs["updates_total"], prometheus.CounterValue, float64(s.Updates), s.SRGName, "update")
		ch <- prometheus.MustNewConstMetric(h.descs["updates_total"], prometheus.CounterValue, float64(s.Deletes), s.SRGName, "delete")
		ch <- prometheus.MustNewConstMetric(h.descs["backlog_size"], prometheus.GaugeValue, float64(s.BacklogDepth), s.SRGName)
		ch <- prometheus.MustNewConstMetric(h.descs["lag_seconds"], prometheus.GaugeValue, s.SyncLagSecs, s.SRGName)
		ch <- prometheus.MustNewConstMetric(h.descs["bulk_total"], prometheus.CounterValue, float64(s.BulkSyncs), s.SRGName)
	}

	return nil
}
