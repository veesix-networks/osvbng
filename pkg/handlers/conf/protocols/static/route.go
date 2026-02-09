package static

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/protocols"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewIPv4RouteHandler)
	conf.RegisterFactory(NewIPv6RouteHandler)
}

type RouteHandler struct {
	pathPattern paths.Path
	callbacks   *conf.Callbacks
}

func NewIPv4RouteHandler(deps *deps.ConfDeps) conf.Handler {
	h := &RouteHandler{
		pathPattern: paths.ProtocolsStaticIPv4Route,
	}
	h.callbacks = &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
	return h
}

func NewIPv6RouteHandler(deps *deps.ConfDeps) conf.Handler {
	h := &RouteHandler{
		pathPattern: paths.ProtocolsStaticIPv6Route,
	}
	h.callbacks = &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
	return h
}

func (h *RouteHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	_, ok := hctx.NewValue.(*protocols.StaticRoute)
	if !ok {
		return fmt.Errorf("expected *protocols.StaticRoute, got %T", hctx.NewValue)
	}
	return nil
}

func (h *RouteHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *RouteHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *RouteHandler) PathPattern() paths.Path {
	return h.pathPattern
}

func (h *RouteHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.Interface}
}

func (h *RouteHandler) Callbacks() *conf.Callbacks {
	return h.callbacks
}
