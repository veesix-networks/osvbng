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
	"github.com/veesix-networks/osvbng/pkg/models"
)

func init() {
	show.RegisterFactory(func(d *deps.ShowDeps) show.ShowHandler {
		return &SchedulerSessionHandler{deps: d}
	})
}

// SchedulerSessionHandler answers "what shaping does this subscriber get":
// the session's scheduler with per-tin stats, and the aggregate tiers above
// it. Deliberately not telemetry-registered - it is an on-demand view.
type SchedulerSessionHandler struct {
	deps *deps.ShowDeps
}

type SchedulerSessionOptions struct {
	SessionID     string `query:"session_id" description:"Subscriber session identifier"`
	AcctSessionID string `query:"acct_session_id" description:"AAA/accounting session identifier (Acct-Session-Id)"`
	Interface     string `query:"interface" description:"Access interface, matched against the session's encap interface or its parent"`
	OuterVLAN     string `query:"outer_vlan" description:"Outer (S-) VLAN, with --interface"`
	InnerVLAN     string `query:"inner_vlan" description:"Inner (C-) VLAN, with --interface"`
}

func (h *SchedulerSessionHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.deps.Southbound == nil {
		return nil, fmt.Errorf("southbound not available")
	}
	if h.deps.Subscriber == nil {
		return nil, fmt.Errorf("subscriber component not available")
	}

	if sessionID := req.Options["session_id"]; sessionID != "" {
		sess, ok := h.deps.Subscriber.SessionSnapshot(ctx, sessionID)
		if !ok {
			return nil, fmt.Errorf("session not found: %s", sessionID)
		}
		return buildSchedulerView(h.deps, sess, sess.GetIfIndex())
	}

	matches, err := h.matchSessions(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(matches) == 1 {
		return buildSchedulerView(h.deps, matches[0], matches[0].GetIfIndex())
	}

	// More than one match: hand back enough identity to re-run unambiguously
	// rather than guessing which subscriber was meant.
	out := &SessionCandidates{
		Note: fmt.Sprintf("%d sessions match, re-run with --session-id", len(matches)),
	}
	for _, sess := range matches {
		cand := SessionCandidate{
			SessionID:     sess.GetSessionID(),
			AcctSessionID: sess.GetAAASessionID(),
			OuterVLAN:     sess.GetOuterVLAN(),
			InnerVLAN:     sess.GetInnerVLAN(),
		}
		if accessIfIndex := sess.GetAccessIfIndex(); accessIfIndex != 0 {
			cand.AccessInterface = interfaceName(h.deps, accessIfIndex)
		}
		out.Candidates = append(out.Candidates, cand)
	}
	return out, nil
}

// matchSessions filters the session store by accounting session id or
// interface+VLANs and
// fails when nothing matches or no filter was given at all.
func (h *SchedulerSessionHandler) matchSessions(ctx context.Context, req *show.Request) ([]models.SubscriberSession, error) {
	acctID := req.Options["acct_session_id"]
	ifName := req.Options["interface"]
	outerStr := req.Options["outer_vlan"]
	innerStr := req.Options["inner_vlan"]

	if acctID == "" && ifName == "" && outerStr == "" {
		return nil, fmt.Errorf("one of --session-id, --acct-session-id, or --interface/--outer-vlan is required")
	}

	var outer, inner uint64
	var innerSet bool
	var err error
	if outerStr != "" {
		if outer, err = strconv.ParseUint(outerStr, 10, 16); err != nil {
			return nil, fmt.Errorf("invalid outer_vlan %q", outerStr)
		}
	}
	if innerStr != "" {
		if inner, err = strconv.ParseUint(innerStr, 10, 16); err != nil {
			return nil, fmt.Errorf("invalid inner_vlan %q", innerStr)
		}
		innerSet = true
	}

	sessions, err := h.deps.Subscriber.GetSessions(ctx, "", "", 0)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	var matches []models.SubscriberSession
	for _, sess := range sessions {
		if acctID != "" && sess.GetAAASessionID() != acctID {
			continue
		}
		if ifName != "" && !h.matchesAccessInterface(sess, ifName) {
			continue
		}
		if outerStr != "" && sess.GetOuterVLAN() != uint16(outer) {
			continue
		}
		if innerSet && sess.GetInnerVLAN() != uint16(inner) {
			continue
		}
		matches = append(matches, sess)
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("no session matches the given filter")
	}
	return matches, nil
}

// matchesAccessInterface accepts either the session's encap sub-interface by
// name or the parent it hangs off, so an operator can name the physical port
// without knowing the per-VLAN sub-interface.
func (h *SchedulerSessionHandler) matchesAccessInterface(sess models.SubscriberSession, name string) bool {
	accessIfIndex := sess.GetAccessIfIndex()
	if accessIfIndex == 0 {
		return false
	}
	mgr := h.deps.Southbound.GetIfMgr()
	iface := mgr.Get(accessIfIndex)
	if iface == nil {
		return false
	}
	if iface.Name == name {
		return true
	}
	if sup := mgr.Get(iface.SupSwIfIndex); sup != nil && sup.Name == name {
		return true
	}
	return false
}

func (h *SchedulerSessionHandler) PathPattern() paths.Path {
	return paths.QoSSchedulerSession
}

func (h *SchedulerSessionHandler) Dependencies() []paths.Path {
	return nil
}

func (h *SchedulerSessionHandler) Summary() string {
	return "Show one subscriber's QoS scheduler and shaping hierarchy"
}

func (h *SchedulerSessionHandler) Description() string {
	return "Look up a session by ID, accounting session id, or interface and VLAN, and display its CAKE scheduler with per-tin stats plus the S-VLAN and port aggregates above it."
}

func (h *SchedulerSessionHandler) OptionsType() interface{} {
	return &SchedulerSessionOptions{}
}
