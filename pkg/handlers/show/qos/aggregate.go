// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package qos

import (
	"context"
	"fmt"
	"strconv"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

func init() {
	show.RegisterFactory(func(d *deps.ShowDeps) show.ShowHandler {
		return &AggregateHandler{deps: d}
	})
	telemetry.RegisterMetric[southbound.AggregateState](paths.QoSAggregate)
}

// AggregateHandler reports both tiers of the shaping hierarchy.
//
// A port and the S-VLANs beneath it are both keyed by the port's sw_if_index,
// so entries are distinguished by their level and tag range rather than by
// interface alone.
type AggregateHandler struct {
	deps *deps.ShowDeps
}

// Collect with no options returns both tiers everywhere: the telemetry
// poller calls this path with an empty request, so filters are subtractive
// only.
func (h *AggregateHandler) Collect(_ context.Context, req *show.Request) (interface{}, error) {
	if h.deps.Southbound == nil {
		return nil, fmt.Errorf("southbound not available")
	}

	aggs, err := h.deps.Southbound.DumpAggregates()
	if err != nil {
		return nil, err
	}

	ifOpt := req.Options["interface"]
	level := req.Options["level"]
	svlanStr := req.Options["svlan"]

	if ifOpt == "" && level == "" && svlanStr == "" {
		return aggs, nil
	}

	var swIfIndex uint32
	if ifOpt != "" {
		if swIfIndex, err = resolveIfIndex(h.deps, ifOpt); err != nil {
			return nil, err
		}
	}
	var svlan uint64
	if svlanStr != "" {
		if svlan, err = strconv.ParseUint(svlanStr, 10, 16); err != nil {
			return nil, fmt.Errorf("invalid svlan %q", svlanStr)
		}
	}

	filtered := aggs[:0]
	for _, a := range aggs {
		if ifOpt != "" && a.SwIfIndex != swIfIndex {
			continue
		}
		if level != "" && a.Level != level {
			continue
		}
		// An S-VLAN aggregate owns a tag range; the filter matches any tag
		// it covers, the way the dataplane itself resolves a tag.
		if svlanStr != "" &&
			(a.Level != "svlan" || uint16(svlan) < a.SVLANID || uint16(svlan) > a.SVLANIDEnd) {
			continue
		}
		filtered = append(filtered, a)
	}

	return filtered, nil
}

type AggregateListOptions struct {
	Interface string `query:"interface" description:"Limit to one port, by name or sw_if_index"`
	Level     string `query:"level" description:"Limit to one tier" enum:"port,svlan"`
	SVLAN     string `query:"svlan" description:"Limit to the S-VLAN aggregate covering this tag"`
}

func (h *AggregateHandler) OptionsType() interface{} {
	return &AggregateListOptions{}
}

func (h *AggregateHandler) PathPattern() paths.Path {
	return paths.QoSAggregate
}

func (h *AggregateHandler) Dependencies() []paths.Path {
	return nil
}

func (h *AggregateHandler) Summary() string {
	return "Show QoS aggregate shapers"
}

func (h *AggregateHandler) Description() string {
	return "Display port and S-VLAN aggregate shapers, their active children, and why children are being held back."
}

func (h *AggregateHandler) SortKey() string {
	return "sw_if_index"
}
