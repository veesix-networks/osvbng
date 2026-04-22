// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package routing_policy

import (
	"context"
	"fmt"
	"net"
	"strings"

	rp "github.com/veesix-networks/osvbng/pkg/config/routing_policy"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewPrefixSetHandler)
	conf.RegisterFactory(NewPrefixSetV6Handler)
}

type PrefixSetHandler struct{}

func NewPrefixSetHandler(deps *deps.ConfDeps) conf.Handler {
	return &PrefixSetHandler{}
}

func (h *PrefixSetHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	ps, ok := hctx.NewValue.(rp.PrefixSet)
	if !ok {
		psPtr, ok := hctx.NewValue.(*rp.PrefixSet)
		if !ok {
			return fmt.Errorf("expected PrefixSet, got %T", hctx.NewValue)
		}
		ps = *psPtr
	}

	for i, entry := range ps {
		if err := validatePrefixEntry(entry, false); err != nil {
			return fmt.Errorf("entry[%d]: %w", i, err)
		}
	}
	return nil
}

func (h *PrefixSetHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *PrefixSetHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *PrefixSetHandler) PathPattern() paths.Path {
	return paths.RoutingPolicyPrefixSet
}

func (h *PrefixSetHandler) Dependencies() []paths.Path {
	return nil
}

func (h *PrefixSetHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}

type PrefixSetV6Handler struct{}

func NewPrefixSetV6Handler(deps *deps.ConfDeps) conf.Handler {
	return &PrefixSetV6Handler{}
}

func (h *PrefixSetV6Handler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	ps, ok := hctx.NewValue.(rp.PrefixSet)
	if !ok {
		psPtr, ok := hctx.NewValue.(*rp.PrefixSet)
		if !ok {
			return fmt.Errorf("expected PrefixSet, got %T", hctx.NewValue)
		}
		ps = *psPtr
	}

	for i, entry := range ps {
		if err := validatePrefixEntry(entry, true); err != nil {
			return fmt.Errorf("entry[%d]: %w", i, err)
		}
	}
	return nil
}

func (h *PrefixSetV6Handler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *PrefixSetV6Handler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *PrefixSetV6Handler) PathPattern() paths.Path {
	return paths.RoutingPolicyPrefixSetV6
}

func (h *PrefixSetV6Handler) Dependencies() []paths.Path {
	return nil
}

func (h *PrefixSetV6Handler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}

func validatePrefixEntry(entry rp.PrefixSetEntry, ipv6 bool) error {
	if entry.Action != "permit" && entry.Action != "deny" {
		return fmt.Errorf("action must be permit or deny, got %q", entry.Action)
	}

	_, network, err := net.ParseCIDR(entry.Prefix)
	if err != nil {
		return fmt.Errorf("invalid prefix %q: %w", entry.Prefix, err)
	}

	if ipv6 && !strings.Contains(entry.Prefix, ":") {
		return fmt.Errorf("expected IPv6 prefix, got %q", entry.Prefix)
	}
	if !ipv6 && strings.Contains(entry.Prefix, ":") {
		return fmt.Errorf("expected IPv4 prefix, got %q", entry.Prefix)
	}

	ones, _ := network.Mask.Size()

	if entry.Le != 0 && int(entry.Le) < ones {
		return fmt.Errorf("le (%d) must be >= prefix length (%d)", entry.Le, ones)
	}
	if entry.Ge != 0 && int(entry.Ge) < ones {
		return fmt.Errorf("ge (%d) must be >= prefix length (%d)", entry.Ge, ones)
	}
	if entry.Le != 0 && entry.Ge != 0 && entry.Le < entry.Ge {
		return fmt.Errorf("le (%d) must be >= ge (%d)", entry.Le, entry.Ge)
	}

	return nil
}

func (h *PrefixSetHandler) Summary() string {
	return "IPv4 prefix list"
}

func (h *PrefixSetHandler) Description() string {
	return "Configure an IPv4 prefix list for route filtering."
}

func (h *PrefixSetHandler) ValueType() interface{} {
	return rp.PrefixSet{}
}

func (h *PrefixSetV6Handler) Summary() string {
	return "IPv6 prefix list"
}

func (h *PrefixSetV6Handler) Description() string {
	return "Configure an IPv6 prefix list for route filtering."
}

func (h *PrefixSetV6Handler) ValueType() interface{} {
	return rp.PrefixSet{}
}
