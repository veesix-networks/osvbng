package static

import (
	"github.com/veesix-networks/osvbng/pkg/deps"
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/operations"
)

func init() {
	conf.RegisterFactory(NewIPv4RouteHandler)
	conf.RegisterFactory(NewIPv6RouteHandler)
}

type RouteHandler struct {
	dataplane   operations.Dataplane
	pathPattern paths.Path
}

func NewIPv4RouteHandler(daemons *deps.ConfDeps) conf.Handler {
	return &RouteHandler{
		dataplane:   daemons.Dataplane,
		pathPattern: paths.ProtocolsStaticIPv4Route,
	}
}

func NewIPv6RouteHandler(daemons *deps.ConfDeps) conf.Handler {
	return &RouteHandler{
		dataplane:   daemons.Dataplane,
		pathPattern: paths.ProtocolsStaticIPv6Route,
	}
}

func (h *RouteHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	_, ok := hctx.NewValue.(*config.StaticRoute)
	if !ok {
		return fmt.Errorf("expected *config.StaticRoute, got %T", hctx.NewValue)
	}
	return nil
}

func (h *RouteHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	route := hctx.NewValue.(*config.StaticRoute)
	return h.dataplane.AddRoute(route)
}

func (h *RouteHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	route := hctx.NewValue.(*config.StaticRoute)
	return h.dataplane.DelRoute(route)
}

func (h *RouteHandler) PathPattern() paths.Path {
	return h.pathPattern
}

func (h *RouteHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.Interface}
}

func (h *RouteHandler) Callbacks() *conf.Callbacks {
	return nil
}
