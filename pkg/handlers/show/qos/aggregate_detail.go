// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package qos

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func init() {
	show.RegisterFactory(func(d *deps.ShowDeps) show.ShowHandler {
		return &AggregateDetailHandler{deps: d}
	})
}

// AggregateDetailHandler renders one port's whole shaping hierarchy as a
// tree: the port aggregate, each S-VLAN aggregate beneath it, and the member
// schedulers of every tier with their stats. The dataplane keeps no child
// lists, so membership comes from the scheduler v2 dump's parent filter.
type AggregateDetailHandler struct {
	deps *deps.ShowDeps
}

type AggregateDetailOptions struct {
	Interface string `query:"interface" description:"Port (physical or bond) interface, by name or sw_if_index"`
	SVLAN     string `query:"svlan" description:"Limit to the S-VLAN aggregate covering this tag"`
}

// AggregateDetail is one tier plus everything beneath it.
type AggregateDetail struct {
	Aggregate  southbound.AggregateState   `json:"aggregate"`
	Children   []AggregateDetail           `json:"children,omitempty"`
	Schedulers []southbound.SchedulerState `json:"schedulers,omitempty"`
	Note       string                      `json:"note,omitempty"`
}

func (h *AggregateDetailHandler) Collect(_ context.Context, req *show.Request) (interface{}, error) {
	if h.deps.Southbound == nil {
		return nil, fmt.Errorf("southbound not available")
	}

	ifOpt := req.Options["interface"]
	if ifOpt == "" {
		return nil, fmt.Errorf("--interface is required")
	}
	swIfIndex, err := resolveIfIndex(h.deps, ifOpt)
	if err != nil {
		return nil, err
	}

	var svlan uint64
	svlanStr := req.Options["svlan"]
	if svlanStr != "" {
		if svlan, err = strconv.ParseUint(svlanStr, 10, 16); err != nil {
			return nil, fmt.Errorf("invalid svlan %q", svlanStr)
		}
	}

	aggs, err := h.deps.Southbound.DumpAggregates()
	if err != nil {
		return nil, err
	}

	var port *southbound.AggregateState
	var svlans []southbound.AggregateState
	for i := range aggs {
		a := aggs[i]
		if a.SwIfIndex != swIfIndex {
			continue
		}
		switch a.Level {
		case "port":
			port = &aggs[i]
		case "svlan":
			if svlanStr != "" && (uint16(svlan) < a.SVLANID || uint16(svlan) > a.SVLANIDEnd) {
				continue
			}
			svlans = append(svlans, a)
		}
	}

	if svlanStr != "" {
		if len(svlans) == 0 {
			return nil, fmt.Errorf("no S-VLAN aggregate covers tag %d on %s", svlan, ifOpt)
		}
		return h.buildTier(svlans[0], nil), nil
	}

	if port == nil {
		if len(svlans) == 0 {
			return nil, fmt.Errorf("no aggregate on interface %s", ifOpt)
		}
		// S-VLANs cannot outlive their port, so this is a transient
		// mid-teardown read; render what exists rather than failing.
		root := &AggregateDetail{Note: "no port aggregate on this interface"}
		for _, sv := range svlans {
			root.Children = append(root.Children, *h.buildTier(sv, nil))
		}
		return root, nil
	}

	return h.buildTier(*port, svlans), nil
}

// buildTier fills one aggregate's members, and for a port its S-VLAN
// children recursively.
func (h *AggregateDetailHandler) buildTier(agg southbound.AggregateState, children []southbound.AggregateState) *AggregateDetail {
	tier := &AggregateDetail{Aggregate: agg}

	svlanID := uint16(0)
	if agg.Level == "svlan" {
		svlanID = agg.SVLANID
	}

	members, err := h.deps.Southbound.DumpSchedulersByParent(agg.SwIfIndex, agg.Level, svlanID)
	switch {
	case errors.Is(err, southbound.ErrSchedV2Unsupported):
		tier.Note = "scheduler membership requires a dataplane with the QoS plugin API >= 3.1.0"
	case err != nil:
		tier.Note = fmt.Sprintf("scheduler membership unavailable: %v", err)
	default:
		enrichSessionIDs(h.deps, members)
		tier.Schedulers = members
	}

	for _, child := range children {
		tier.Children = append(tier.Children, *h.buildTier(child, nil))
	}

	return tier
}

func (h *AggregateDetailHandler) PathPattern() paths.Path {
	return paths.QoSAggregateDetail
}

func (h *AggregateDetailHandler) Dependencies() []paths.Path {
	return nil
}

func (h *AggregateDetailHandler) Summary() string {
	return "Show a port's QoS shaping hierarchy"
}

func (h *AggregateDetailHandler) Description() string {
	return "Display a port aggregate, its S-VLAN aggregates, and the member schedulers of every tier with their stats, as one tree."
}

func (h *AggregateDetailHandler) OptionsType() interface{} {
	return &AggregateDetailOptions{}
}
