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
	Register(paths.HAStatus.String(), func(logger *logger.Logger) (MetricHandler, error) {
		return newHAMetricHandler(logger), nil
	})
}

type haStatusMetric struct {
	Enabled bool `json:"enabled"`
	NodeID  string `json:"node_id,omitempty"`
	Peer    *haPeerMetric `json:"peer,omitempty"`
	SRGs    []haSRGMetric `json:"srgs,omitempty"`
}

type haPeerMetric struct {
	Connected bool   `json:"connected"`
	NodeID    string `json:"node_id,omitempty"`
}

type haSRGMetric struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

type haMetricHandler struct {
	logger *logger.Logger
	descs  map[string]*prometheus.Desc
}

func newHAMetricHandler(logger *logger.Logger) *haMetricHandler {
	srgLabels := []string{"srg", "state"}
	return &haMetricHandler{
		logger: logger,
		descs: map[string]*prometheus.Desc{
			"srg_state":      prometheus.NewDesc("osvbng_ha_srg_state", "SRG state (1 for current state)", srgLabels, nil),
			"peer_connected": prometheus.NewDesc("osvbng_ha_peer_connected", "Whether the HA peer is connected (1) or not (0)", nil, nil),
		},
	}
}

func (h *haMetricHandler) Name() string {
	return paths.HAStatus.String()
}

func (h *haMetricHandler) Paths() []string {
	return []string{paths.HAStatus.String()}
}

func (h *haMetricHandler) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range h.descs {
		ch <- desc
	}
}

func (h *haMetricHandler) Collect(ctx context.Context, store cache.Cache, ch chan<- prometheus.Metric) error {
	fullPath := "osvbng:state:" + paths.HAStatus.String()

	data, err := store.Get(ctx, fullPath)
	if err != nil {
		return nil
	}

	var status haStatusMetric
	if err := json.Unmarshal(data, &status); err != nil {
		return err
	}

	if !status.Enabled {
		return nil
	}

	if status.Peer != nil {
		var connected float64
		if status.Peer.Connected {
			connected = 1
		}
		ch <- prometheus.MustNewConstMetric(h.descs["peer_connected"], prometheus.GaugeValue, connected)
	}

	for _, srg := range status.SRGs {
		ch <- prometheus.MustNewConstMetric(h.descs["srg_state"], prometheus.GaugeValue, 1, srg.Name, srg.State)
	}

	return nil
}
