// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package routing_policy

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"

	rp "github.com/veesix-networks/osvbng/pkg/config/routing_policy"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewRoutePolicyHandler)
}

type RoutePolicyHandler struct{}

func NewRoutePolicyHandler(deps *deps.ConfDeps) conf.Handler {
	return &RoutePolicyHandler{}
}

func (h *RoutePolicyHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	policy, ok := hctx.NewValue.(rp.RoutePolicy)
	if !ok {
		policyPtr, ok := hctx.NewValue.(*rp.RoutePolicy)
		if !ok {
			return fmt.Errorf("expected RoutePolicy, got %T", hctx.NewValue)
		}
		policy = *policyPtr
	}

	if hctx.Config == nil || hctx.Config.RoutingPolicies == nil {
		return fmt.Errorf("routing-policies config not available for cross-reference validation")
	}
	rpCfg := hctx.Config.RoutingPolicies

	seqs := make(map[uint32]bool)
	for i, entry := range policy {
		if entry.Sequence == 0 {
			return fmt.Errorf("entry[%d]: sequence must be > 0", i)
		}
		if seqs[entry.Sequence] {
			return fmt.Errorf("entry[%d]: duplicate sequence %d", i, entry.Sequence)
		}
		seqs[entry.Sequence] = true

		if entry.Action != "permit" && entry.Action != "deny" {
			return fmt.Errorf("entry[%d]: action must be permit or deny, got %q", i, entry.Action)
		}

		if entry.Match != nil {
			if err := validateMatch(entry.Match, rpCfg, i); err != nil {
				return err
			}
		}

		if entry.Set != nil {
			if err := validateSet(entry.Set, rpCfg, i); err != nil {
				return err
			}
		}

		if entry.OnMatch != "" {
			if err := validateOnMatch(entry.OnMatch, i); err != nil {
				return err
			}
		}
	}

	// Extract this policy's name from the path for cycle detection
	wildcards, err := paths.RoutingPolicyRoutePolicy.ExtractWildcards(hctx.Path, 1)
	if err == nil && len(wildcards) > 0 {
		policyName := wildcards[0]
		for i, entry := range policy {
			if entry.Call != "" {
				if _, exists := rpCfg.RoutePolicies[entry.Call]; !exists {
					return fmt.Errorf("entry[%d]: call target %q does not exist", i, entry.Call)
				}
				if err := detectCallCycle(policyName, entry.Call, rpCfg.RoutePolicies, nil); err != nil {
					return fmt.Errorf("entry[%d]: %w", i, err)
				}
			}
		}
	}

	return nil
}

func validateMatch(m *rp.RoutePolicyMatch, rpCfg *rp.RoutingPolicyConfig, idx int) error {
	if m.PrefixSet != "" {
		if _, exists := rpCfg.PrefixSets[m.PrefixSet]; !exists {
			return fmt.Errorf("entry[%d]: prefix-set %q does not exist in prefix-sets", idx, m.PrefixSet)
		}
	}
	if m.PrefixSetV6 != "" {
		if _, exists := rpCfg.PrefixSetsV6[m.PrefixSetV6]; !exists {
			return fmt.Errorf("entry[%d]: prefix-set-v6 %q does not exist in prefix-sets-v6", idx, m.PrefixSetV6)
		}
	}
	if m.CommunitySet != "" {
		if _, exists := rpCfg.CommunitySets[m.CommunitySet]; !exists {
			return fmt.Errorf("entry[%d]: community-set %q does not exist", idx, m.CommunitySet)
		}
	}
	if m.ExtCommunitySet != "" {
		if _, exists := rpCfg.ExtCommunitySets[m.ExtCommunitySet]; !exists {
			return fmt.Errorf("entry[%d]: ext-community-set %q does not exist", idx, m.ExtCommunitySet)
		}
	}
	if m.LargeCommunitySet != "" {
		if _, exists := rpCfg.LargeCommunitySets[m.LargeCommunitySet]; !exists {
			return fmt.Errorf("entry[%d]: large-community-set %q does not exist", idx, m.LargeCommunitySet)
		}
	}
	if m.ASPathSet != "" {
		if _, exists := rpCfg.ASPathSets[m.ASPathSet]; !exists {
			return fmt.Errorf("entry[%d]: as-path-set %q does not exist", idx, m.ASPathSet)
		}
	}
	return nil
}

func validateSet(s *rp.RoutePolicySet, rpCfg *rp.RoutingPolicyConfig, idx int) error {
	if s.Origin != "" {
		switch s.Origin {
		case "igp", "egp", "incomplete":
		default:
			return fmt.Errorf("entry[%d]: origin must be igp, egp, or incomplete, got %q", idx, s.Origin)
		}
	}

	if s.CommunityDelete != "" {
		if _, exists := rpCfg.CommunitySets[s.CommunityDelete]; !exists {
			return fmt.Errorf("entry[%d]: community-delete references non-existent community-set %q", idx, s.CommunityDelete)
		}
	}

	if s.NextHopIPv4 != "" {
		if ip := net.ParseIP(s.NextHopIPv4); ip == nil || ip.To4() == nil {
			return fmt.Errorf("entry[%d]: next-hop-ipv4 %q is not a valid IPv4 address", idx, s.NextHopIPv4)
		}
	}

	if s.NextHopIPv6 != "" {
		if ip := net.ParseIP(s.NextHopIPv6); ip == nil || ip.To4() != nil {
			return fmt.Errorf("entry[%d]: next-hop-ipv6 %q is not a valid IPv6 address", idx, s.NextHopIPv6)
		}
	}

	return nil
}

func validateOnMatch(onMatch string, idx int) error {
	if onMatch == "next" {
		return nil
	}
	if strings.HasPrefix(onMatch, "goto ") {
		numStr := strings.TrimPrefix(onMatch, "goto ")
		if _, err := strconv.ParseUint(numStr, 10, 32); err != nil {
			return fmt.Errorf("entry[%d]: invalid on-match goto value %q", idx, numStr)
		}
		return nil
	}
	return fmt.Errorf("entry[%d]: on-match must be 'next' or 'goto N', got %q", idx, onMatch)
}

func detectCallCycle(origin, current string, policies map[string]rp.RoutePolicy, visited map[string]bool) error {
	if visited == nil {
		visited = make(map[string]bool)
	}
	if current == origin {
		return fmt.Errorf("call cycle detected: policy %q eventually calls back to itself", origin)
	}
	if visited[current] {
		return nil
	}
	visited[current] = true

	policy, exists := policies[current]
	if !exists {
		return nil
	}
	for _, entry := range policy {
		if entry.Call != "" {
			if err := detectCallCycle(origin, entry.Call, policies, visited); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *RoutePolicyHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *RoutePolicyHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *RoutePolicyHandler) PathPattern() paths.Path {
	return paths.RoutingPolicyRoutePolicy
}

func (h *RoutePolicyHandler) Dependencies() []paths.Path {
	return nil
}

func (h *RoutePolicyHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}

func (h *RoutePolicyHandler) Summary() string {
	return "Route policy (route-map)"
}

func (h *RoutePolicyHandler) Description() string {
	return "Configure a route policy with match and set clauses."
}

func (h *RoutePolicyHandler) ValueType() interface{} {
	return rp.RoutePolicy{}
}
