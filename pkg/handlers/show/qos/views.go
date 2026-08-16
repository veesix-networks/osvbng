// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package qos

import (
	"fmt"
	"strconv"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

// SchedulerSessionView is one subscriber's whole shaping chain: the session
// identity, its CAKE scheduler, and the aggregate tiers above it.
type SchedulerSessionView struct {
	SessionID       string `json:"session_id,omitempty"`
	AcctSessionID   string `json:"acct_session_id,omitempty"`
	AccessType      string `json:"access_type,omitempty"`
	ServiceGroup    string `json:"service_group,omitempty"`
	Interface       string `json:"interface,omitempty"`
	AccessInterface string `json:"access_interface,omitempty"`
	OuterVLAN       uint16 `json:"outer_vlan,omitempty"`
	InnerVLAN       uint16 `json:"inner_vlan,omitempty"`

	Scheduler   *southbound.SchedulerState `json:"scheduler,omitempty"`
	ParentSVLAN *southbound.AggregateState `json:"parent_svlan,omitempty"`
	ParentPort  *southbound.AggregateState `json:"parent_port,omitempty"`

	Note string `json:"note,omitempty"`
}

// SessionCandidates is the answer to a filter that matched more than one
// session: enough identity to re-run the lookup unambiguously.
type SessionCandidates struct {
	Note       string             `json:"note"`
	Candidates []SessionCandidate `json:"candidates"`
}

type SessionCandidate struct {
	SessionID       string `json:"session_id"`
	AcctSessionID   string `json:"acct_session_id,omitempty"`
	AccessInterface string `json:"access_interface,omitempty"`
	OuterVLAN       uint16 `json:"outer_vlan"`
	InnerVLAN       uint16 `json:"inner_vlan"`
}

// resolveIfIndex accepts an interface name or a numeric sw_if_index.
func resolveIfIndex(d *deps.ShowDeps, s string) (uint32, error) {
	if n, err := strconv.ParseUint(s, 10, 32); err == nil {
		return uint32(n), nil
	}
	if idx, ok := d.Southbound.GetIfMgr().GetSwIfIndex(s); ok {
		return idx, nil
	}
	return 0, fmt.Errorf("unknown interface %q", s)
}

// interfaceName resolves a sw_if_index to a name, empty when unknown.
func interfaceName(d *deps.ShowDeps, swIfIndex uint32) string {
	if iface := d.Southbound.GetIfMgr().Get(swIfIndex); iface != nil {
		return iface.Name
	}
	return ""
}

// enrichSessionIDs is best-effort: rows whose interface is not a session
// interface, or whose session raced teardown, keep an empty session_id.
func enrichSessionIDs(d *deps.ShowDeps, states []southbound.SchedulerState) {
	if d.Subscriber == nil {
		return
	}
	for i := range states {
		if id, ok := d.Subscriber.SessionIDByIfIndex(states[i].SwIfIndex); ok {
			states[i].SessionID = id
		}
	}
}

// findAggregate matches the (sw_if_index, level, first tag) composite key.
func findAggregate(aggs []southbound.AggregateState, swIfIndex uint32, level string, svlanID uint16) *southbound.AggregateState {
	for i := range aggs {
		a := &aggs[i]
		if a.SwIfIndex != swIfIndex || a.Level != level {
			continue
		}
		if level == "svlan" && a.SVLANID != svlanID {
			continue
		}
		return a
	}
	return nil
}

// buildSchedulerView assembles the per-session chain. sess may be nil when
// the caller starts from an interface the reverse index cannot resolve.
func buildSchedulerView(d *deps.ShowDeps, sess models.SubscriberSession, swIfIndex uint32) (*SchedulerSessionView, error) {
	view := &SchedulerSessionView{
		Interface: interfaceName(d, swIfIndex),
	}

	if sess != nil {
		view.SessionID = sess.GetSessionID()
		view.AcctSessionID = sess.GetAAASessionID()
		view.AccessType = string(sess.GetAccessType())
		view.ServiceGroup = sess.GetServiceGroup()
		view.OuterVLAN = sess.GetOuterVLAN()
		view.InnerVLAN = sess.GetInnerVLAN()
		// Zero means "not recorded" (L2TP has no access interface at this
		// layer), and must not render as local0.
		if accessIfIndex := sess.GetAccessIfIndex(); accessIfIndex != 0 {
			view.AccessInterface = interfaceName(d, accessIfIndex)
		}
	} else if d.Subscriber != nil {
		if id, ok := d.Subscriber.SessionIDByIfIndex(swIfIndex); ok {
			view.SessionID = id
		}
	}

	sched, err := d.Southbound.DumpScheduler(swIfIndex)
	if err != nil {
		return nil, fmt.Errorf("dump scheduler: %w", err)
	}
	if sched == nil {
		view.Note = "no QoS scheduler on this interface"
		return view, nil
	}
	if sched.SessionID == "" {
		sched.SessionID = view.SessionID
	}
	view.Scheduler = sched

	if !sched.HasParent {
		return view, nil
	}

	// The parent identity is the aggregate dump's own composite key, so one
	// dump resolves both tiers.
	aggs, err := d.Southbound.DumpAggregates()
	if err != nil {
		return nil, fmt.Errorf("dump aggregates: %w", err)
	}
	if sched.ParentLevel == "svlan" {
		view.ParentSVLAN = findAggregate(aggs, sched.ParentSwIfIndex, "svlan", sched.ParentSVLANID)
	}
	view.ParentPort = findAggregate(aggs, sched.ParentSwIfIndex, "port", 0)

	return view, nil
}
