package isis

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewISISDomainPasswordHandler)
}

type ISISDomainPasswordHandler struct{}

func NewISISDomainPasswordHandler(deps *deps.ConfDeps) conf.Handler {
	return &ISISDomainPasswordHandler{}
}

func (h *ISISDomainPasswordHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	if _, ok := hctx.NewValue.(string); !ok {
		return fmt.Errorf("expected string, got %T", hctx.NewValue)
	}
	return nil
}

func (h *ISISDomainPasswordHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISDomainPasswordHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISDomainPasswordHandler) PathPattern() paths.Path {
	return paths.ISISDomainPassword
}

func (h *ISISDomainPasswordHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ISISEnabled}
}

func (h *ISISDomainPasswordHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
