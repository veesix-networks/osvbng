package static

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/conf/handlers"
	"github.com/veesix-networks/osvbng/pkg/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/conf/types"
	"github.com/veesix-networks/osvbng/pkg/operations"
)

func init() {
	handlers.RegisterFactory(NewIPv4RouteHandler)
	handlers.RegisterFactory(NewIPv6RouteHandler)
}

type RouteHandler struct {
	dataplane   operations.Dataplane
	pathPattern paths.Path
}

func NewIPv4RouteHandler(daemons *handlers.ConfDeps) handlers.Handler {
	return &RouteHandler{
		dataplane:   daemons.Dataplane,
		pathPattern: "protocols.static.ipv4.*",
	}
}

func NewIPv6RouteHandler(daemons *handlers.ConfDeps) handlers.Handler {
	return &RouteHandler{
		dataplane:   daemons.Dataplane,
		pathPattern: "protocols.static.ipv6.*",
	}
}

func (h *RouteHandler) Validate(ctx context.Context, hctx *handlers.HandlerContext) error {
	_, ok := hctx.NewValue.(*types.StaticRoute)
	if !ok {
		return fmt.Errorf("expected *types.StaticRoute, got %T", hctx.NewValue)
	}
	return nil
}

func (h *RouteHandler) Apply(ctx context.Context, hctx *handlers.HandlerContext) error {
	route := hctx.NewValue.(*types.StaticRoute)
	return h.dataplane.AddRoute(route)
}

func (h *RouteHandler) Rollback(ctx context.Context, hctx *handlers.HandlerContext) error {
	route := hctx.NewValue.(*types.StaticRoute)
	return h.dataplane.DelRoute(route)
}

func (h *RouteHandler) PathPattern() paths.Path {
	return h.pathPattern
}

func (h *RouteHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.Interface}
}

func (h *RouteHandler) Callbacks() *handlers.Callbacks {
	return nil
}
