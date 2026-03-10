// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package metrics

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/dhcp/relay"
	"github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	Register(paths.DHCPRelay.String(), func(logger *slog.Logger) (MetricHandler, error) {
		return newDHCPRelayMetricHandler(logger), nil
	})

	Register(paths.DHCPProxy.String(), func(logger *slog.Logger) (MetricHandler, error) {
		return newDHCPProxyMetricHandler(logger), nil
	})
}

type dhcpRelayMetricHandler struct {
	logger *slog.Logger
	descs  map[string]*prometheus.Desc
}

type dhcpRelayData struct {
	Stats   relay.ClientStats    `json:"stats"`
	Servers []relay.ServerStatus `json:"servers"`
}

func newDHCPRelayMetricHandler(logger *slog.Logger) *dhcpRelayMetricHandler {
	serverLabels := []string{"address"}
	return &dhcpRelayMetricHandler{
		logger: logger,
		descs: map[string]*prometheus.Desc{
			"requests_v4": prometheus.NewDesc("osvbng_dhcp_relay_requests_v4", "DHCPv4 relay requests forwarded", nil, nil),
			"replies_v4":  prometheus.NewDesc("osvbng_dhcp_relay_replies_v4", "DHCPv4 relay replies received", nil, nil),
			"timeouts_v4": prometheus.NewDesc("osvbng_dhcp_relay_timeouts_v4", "DHCPv4 relay server timeouts", nil, nil),
			"requests_v6": prometheus.NewDesc("osvbng_dhcp_relay_requests_v6", "DHCPv6 relay requests forwarded", nil, nil),
			"replies_v6":  prometheus.NewDesc("osvbng_dhcp_relay_replies_v6", "DHCPv6 relay replies received", nil, nil),
			"timeouts_v6": prometheus.NewDesc("osvbng_dhcp_relay_timeouts_v6", "DHCPv6 relay server timeouts", nil, nil),
			"server_requests": prometheus.NewDesc("osvbng_dhcp_relay_server_requests", "Requests sent to DHCP relay server", serverLabels, nil),
			"server_timeouts": prometheus.NewDesc("osvbng_dhcp_relay_server_timeouts", "Timeouts from DHCP relay server", serverLabels, nil),
			"server_dead":     prometheus.NewDesc("osvbng_dhcp_relay_server_dead", "Whether DHCP relay server is marked dead", serverLabels, nil),
		},
	}
}

func (h *dhcpRelayMetricHandler) Name() string {
	return paths.DHCPRelay.String()
}

func (h *dhcpRelayMetricHandler) Paths() []string {
	return []string{paths.DHCPRelay.String()}
}

func (h *dhcpRelayMetricHandler) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range h.descs {
		ch <- desc
	}
}

func (h *dhcpRelayMetricHandler) Collect(ctx context.Context, store cache.Cache, ch chan<- prometheus.Metric) error {
	data, err := store.Get(ctx, "osvbng:state:"+paths.DHCPRelay.String())
	if err != nil {
		return nil
	}

	var info dhcpRelayData
	if err := json.Unmarshal(data, &info); err != nil {
		return err
	}

	ch <- prometheus.MustNewConstMetric(h.descs["requests_v4"], prometheus.CounterValue, float64(info.Stats.Requests4))
	ch <- prometheus.MustNewConstMetric(h.descs["replies_v4"], prometheus.CounterValue, float64(info.Stats.Replies4))
	ch <- prometheus.MustNewConstMetric(h.descs["timeouts_v4"], prometheus.CounterValue, float64(info.Stats.Timeouts4))
	ch <- prometheus.MustNewConstMetric(h.descs["requests_v6"], prometheus.CounterValue, float64(info.Stats.Requests6))
	ch <- prometheus.MustNewConstMetric(h.descs["replies_v6"], prometheus.CounterValue, float64(info.Stats.Replies6))
	ch <- prometheus.MustNewConstMetric(h.descs["timeouts_v6"], prometheus.CounterValue, float64(info.Stats.Timeouts6))

	for _, srv := range info.Servers {
		ch <- prometheus.MustNewConstMetric(h.descs["server_requests"], prometheus.CounterValue, float64(srv.Requests), srv.Address)
		ch <- prometheus.MustNewConstMetric(h.descs["server_timeouts"], prometheus.CounterValue, float64(srv.Timeouts), srv.Address)
		var dead float64
		if srv.Dead {
			dead = 1
		}
		ch <- prometheus.MustNewConstMetric(h.descs["server_dead"], prometheus.GaugeValue, dead, srv.Address)
	}

	return nil
}

type dhcpProxyMetricHandler struct {
	logger *slog.Logger
	descs  map[string]*prometheus.Desc
}

type dhcpProxyData struct {
	V4Bindings int `json:"v4Bindings"`
	V6Bindings int `json:"v6Bindings"`
}

func newDHCPProxyMetricHandler(logger *slog.Logger) *dhcpProxyMetricHandler {
	return &dhcpProxyMetricHandler{
		logger: logger,
		descs: map[string]*prometheus.Desc{
			"bindings_v4": prometheus.NewDesc("osvbng_dhcp_proxy_bindings_v4", "Active DHCPv4 proxy bindings", nil, nil),
			"bindings_v6": prometheus.NewDesc("osvbng_dhcp_proxy_bindings_v6", "Active DHCPv6 proxy bindings", nil, nil),
		},
	}
}

func (h *dhcpProxyMetricHandler) Name() string {
	return paths.DHCPProxy.String()
}

func (h *dhcpProxyMetricHandler) Paths() []string {
	return []string{paths.DHCPProxy.String()}
}

func (h *dhcpProxyMetricHandler) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range h.descs {
		ch <- desc
	}
}

func (h *dhcpProxyMetricHandler) Collect(ctx context.Context, store cache.Cache, ch chan<- prometheus.Metric) error {
	data, err := store.Get(ctx, "osvbng:state:"+paths.DHCPProxy.String())
	if err != nil {
		return nil
	}

	var info dhcpProxyData
	if err := json.Unmarshal(data, &info); err != nil {
		return err
	}

	ch <- prometheus.MustNewConstMetric(h.descs["bindings_v4"], prometheus.GaugeValue, float64(info.V4Bindings))
	ch <- prometheus.MustNewConstMetric(h.descs["bindings_v6"], prometheus.GaugeValue, float64(info.V6Bindings))

	return nil
}
