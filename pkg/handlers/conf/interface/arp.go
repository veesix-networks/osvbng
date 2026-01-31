package iface

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func init() {
	conf.RegisterFactory(NewInterfaceARPHandler)
	conf.RegisterFactory(NewSubinterfaceARPHandler)
}

type ARPHandler struct {
	southbound   *southbound.VPP
	pathPattern  paths.Path
	dependencies []paths.Path
}

func NewInterfaceARPHandler(d *deps.ConfDeps) conf.Handler {
	return &ARPHandler{
		southbound:   d.Southbound,
		pathPattern:  paths.InterfaceARP,
		dependencies: []paths.Path{paths.Interface},
	}
}

func NewSubinterfaceARPHandler(d *deps.ConfDeps) conf.Handler {
	return &ARPHandler{
		southbound:   d.Southbound,
		pathPattern:  paths.InterfaceSubinterfaceARP,
		dependencies: []paths.Path{paths.InterfaceSubinterface},
	}
}

func (h *ARPHandler) extractInterfaceName(path string) (string, error) {
	values, err := h.pathPattern.ExtractWildcards(path, 2)
	if err != nil {
		return "", err
	}
	if len(values) == 1 {
		return values[0], nil
	}
	return fmt.Sprintf("%s.%s", values[0], values[1]), nil
}

func (h *ARPHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	_, ok := hctx.NewValue.(*interfaces.ARPConfig)
	if !ok {
		return fmt.Errorf("expected *interfaces.ARPConfig, got %T", hctx.NewValue)
	}
	return nil
}

func (h *ARPHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg := hctx.NewValue.(*interfaces.ARPConfig)

	if cfg.Enabled {
		return nil
	}

	ifName, err := h.extractInterfaceName(hctx.Path)
	if err != nil {
		return fmt.Errorf("extract interface name: %w", err)
	}

	return h.southbound.DisableARPReply(ifName)
}

func (h *ARPHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ARPHandler) PathPattern() paths.Path {
	return h.pathPattern
}

func (h *ARPHandler) Dependencies() []paths.Path {
	return h.dependencies
}

func (h *ARPHandler) Callbacks() *conf.Callbacks {
	return nil
}
