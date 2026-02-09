package ospf

import (
	"context"
	"fmt"
	"net"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewOSPFRouterIDHandler)
}

type OSPFRouterIDHandler struct{}

func NewOSPFRouterIDHandler(deps *deps.ConfDeps) conf.Handler {
	return &OSPFRouterIDHandler{}
}

func (h *OSPFRouterIDHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	val, ok := hctx.NewValue.(string)
	if !ok {
		return fmt.Errorf("expected string, got %T", hctx.NewValue)
	}

	if val != "" && net.ParseIP(val) == nil {
		return fmt.Errorf("invalid router-id %q: must be A.B.C.D format", val)
	}

	return nil
}

func (h *OSPFRouterIDHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPFRouterIDHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPFRouterIDHandler) PathPattern() paths.Path {
	return paths.OSPFRouterID
}

func (h *OSPFRouterIDHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.OSPFEnabled}
}

func (h *OSPFRouterIDHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
