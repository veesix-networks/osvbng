// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package routing_policy

import (
	"context"
	"fmt"
	"regexp"

	rp "github.com/veesix-networks/osvbng/pkg/config/routing_policy"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

var (
	communityRegex = regexp.MustCompile(`^\d+:\d+$`)
	wellKnownNames = map[string]bool{
		"no-export":    true,
		"no-advertise": true,
		"no-peer":      true,
		"blackhole":    true,
		"local-AS":     true,
		"internet":     true,
	}
)

func init() {
	conf.RegisterFactory(NewCommunitySetHandler)
}

type CommunitySetHandler struct{}

func NewCommunitySetHandler(deps *deps.ConfDeps) conf.Handler {
	return &CommunitySetHandler{}
}

func (h *CommunitySetHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	cs, ok := hctx.NewValue.(rp.CommunitySet)
	if !ok {
		csPtr, ok := hctx.NewValue.(*rp.CommunitySet)
		if !ok {
			return fmt.Errorf("expected CommunitySet, got %T", hctx.NewValue)
		}
		cs = *csPtr
	}

	for i, member := range cs {
		if !communityRegex.MatchString(member) && !wellKnownNames[member] {
			return fmt.Errorf("community-set[%d]: invalid community %q (expected AA:NN or well-known name)", i, member)
		}
	}
	return nil
}

func (h *CommunitySetHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *CommunitySetHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *CommunitySetHandler) PathPattern() paths.Path {
	return paths.RoutingPolicyCommunitySet
}

func (h *CommunitySetHandler) Dependencies() []paths.Path {
	return nil
}

func (h *CommunitySetHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}

func (h *CommunitySetHandler) Summary() string {
	return "BGP community list"
}

func (h *CommunitySetHandler) Description() string {
	return "Configure a BGP community list for route matching."
}

func (h *CommunitySetHandler) ValueType() interface{} {
	return rp.CommunitySet{}
}
