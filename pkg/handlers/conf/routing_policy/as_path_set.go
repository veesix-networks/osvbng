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

func init() {
	conf.RegisterFactory(NewASPathSetHandler)
}

type ASPathSetHandler struct{}

func NewASPathSetHandler(deps *deps.ConfDeps) conf.Handler {
	return &ASPathSetHandler{}
}

func (h *ASPathSetHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	as, ok := hctx.NewValue.(rp.ASPathSet)
	if !ok {
		asPtr, ok := hctx.NewValue.(*rp.ASPathSet)
		if !ok {
			return fmt.Errorf("expected ASPathSet, got %T", hctx.NewValue)
		}
		as = *asPtr
	}

	for i, entry := range as {
		if entry.Action != "permit" && entry.Action != "deny" {
			return fmt.Errorf("as-path-set[%d]: action must be permit or deny, got %q", i, entry.Action)
		}
		if _, err := regexp.Compile(entry.Regex); err != nil {
			return fmt.Errorf("as-path-set[%d]: invalid regex %q: %w", i, entry.Regex, err)
		}
	}
	return nil
}

func (h *ASPathSetHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ASPathSetHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ASPathSetHandler) PathPattern() paths.Path {
	return paths.RoutingPolicyASPathSet
}

func (h *ASPathSetHandler) Dependencies() []paths.Path {
	return nil
}

func (h *ASPathSetHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
