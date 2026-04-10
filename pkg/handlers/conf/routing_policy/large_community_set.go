// Copyright 2026 Veesix Networks Ltd
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

var largeCommunityRegex = regexp.MustCompile(`^\d+:\d+:\d+$`)

func init() {
	conf.RegisterFactory(NewLargeCommunitySetHandler)
}

type LargeCommunitySetHandler struct{}

func NewLargeCommunitySetHandler(deps *deps.ConfDeps) conf.Handler {
	return &LargeCommunitySetHandler{}
}

func (h *LargeCommunitySetHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	lcs, ok := hctx.NewValue.(rp.LargeCommunitySet)
	if !ok {
		lcsPtr, ok := hctx.NewValue.(*rp.LargeCommunitySet)
		if !ok {
			return fmt.Errorf("expected LargeCommunitySet, got %T", hctx.NewValue)
		}
		lcs = *lcsPtr
	}

	for i, member := range lcs {
		if !largeCommunityRegex.MatchString(member) {
			return fmt.Errorf("large-community-set[%d]: invalid format %q (expected GLOBAL:LOCAL1:LOCAL2)", i, member)
		}
	}
	return nil
}

func (h *LargeCommunitySetHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *LargeCommunitySetHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *LargeCommunitySetHandler) PathPattern() paths.Path {
	return paths.RoutingPolicyLargeCommunitySet
}

func (h *LargeCommunitySetHandler) Dependencies() []paths.Path {
	return nil
}

func (h *LargeCommunitySetHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}

func (h *LargeCommunitySetHandler) Summary() string {
	return "BGP large community list"
}

func (h *LargeCommunitySetHandler) Description() string {
	return "Configure a BGP large community list for route matching."
}

func (h *LargeCommunitySetHandler) ValueType() interface{} {
	return rp.LargeCommunitySet{}
}
