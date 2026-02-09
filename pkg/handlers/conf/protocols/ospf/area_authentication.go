package ospf

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/protocols"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewOSPFAreaAuthenticationHandler)
}

type OSPFAreaAuthenticationHandler struct{}

func NewOSPFAreaAuthenticationHandler(deps *deps.ConfDeps) conf.Handler {
	return &OSPFAreaAuthenticationHandler{}
}

func (h *OSPFAreaAuthenticationHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	val, ok := hctx.NewValue.(protocols.OSPFAuthMode)
	if !ok {
		return fmt.Errorf("expected protocols.OSPFAuthMode, got %T", hctx.NewValue)
	}

	if !val.Valid() {
		return fmt.Errorf("invalid authentication mode %q", val)
	}

	return nil
}

func (h *OSPFAreaAuthenticationHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPFAreaAuthenticationHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPFAreaAuthenticationHandler) PathPattern() paths.Path {
	return paths.OSPFAreaAuthentication
}

func (h *OSPFAreaAuthenticationHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.OSPFEnabled}
}

func (h *OSPFAreaAuthenticationHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
