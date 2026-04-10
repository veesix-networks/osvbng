// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package routing_policy

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	rp "github.com/veesix-networks/osvbng/pkg/config/routing_policy"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

var (
	extCommunityTypes = map[string]bool{"rt": true, "soo": true}
	// Matches AA:NN (2-byte AS), AS4:NN (4-byte AS), A.B.C.D:NN (IPv4 address)
	extCommunityValueRegex = regexp.MustCompile(`^(\d+:\d+|\d+\.\d+\.\d+\.\d+:\d+)$`)
)

func init() {
	conf.RegisterFactory(NewExtCommunitySetHandler)
}

type ExtCommunitySetHandler struct{}

func NewExtCommunitySetHandler(deps *deps.ConfDeps) conf.Handler {
	return &ExtCommunitySetHandler{}
}

func (h *ExtCommunitySetHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	ecs, ok := hctx.NewValue.(rp.ExtCommunitySet)
	if !ok {
		ecsPtr, ok := hctx.NewValue.(*rp.ExtCommunitySet)
		if !ok {
			return fmt.Errorf("expected ExtCommunitySet, got %T", hctx.NewValue)
		}
		ecs = *ecsPtr
	}

	for i, member := range ecs {
		parts := strings.SplitN(member, " ", 2)
		if len(parts) != 2 {
			return fmt.Errorf("ext-community-set[%d]: expected '<type> <value>', got %q", i, member)
		}
		if !extCommunityTypes[parts[0]] {
			return fmt.Errorf("ext-community-set[%d]: unknown type %q (expected rt or soo)", i, parts[0])
		}
		if !extCommunityValueRegex.MatchString(parts[1]) {
			return fmt.Errorf("ext-community-set[%d]: invalid value %q (expected AA:NN, AS4:NN, or A.B.C.D:NN)", i, parts[1])
		}
	}
	return nil
}

func (h *ExtCommunitySetHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ExtCommunitySetHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ExtCommunitySetHandler) PathPattern() paths.Path {
	return paths.RoutingPolicyExtCommunitySet
}

func (h *ExtCommunitySetHandler) Dependencies() []paths.Path {
	return nil
}

func (h *ExtCommunitySetHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}

func (h *ExtCommunitySetHandler) Summary() string {
	return "BGP extended community list"
}

func (h *ExtCommunitySetHandler) Description() string {
	return "Configure a BGP extended community list for route matching."
}

func (h *ExtCommunitySetHandler) ValueType() interface{} {
	return rp.ExtCommunitySet{}
}
