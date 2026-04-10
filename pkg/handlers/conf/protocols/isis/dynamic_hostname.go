package isis

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewISISDynamicHostnameHandler)
}

type ISISDynamicHostnameHandler struct{}

func NewISISDynamicHostnameHandler(deps *deps.ConfDeps) conf.Handler {
	return &ISISDynamicHostnameHandler{}
}

func (h *ISISDynamicHostnameHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	if _, ok := hctx.NewValue.(bool); !ok {
		return fmt.Errorf("expected bool, got %T", hctx.NewValue)
	}
	return nil
}

func (h *ISISDynamicHostnameHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISDynamicHostnameHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISDynamicHostnameHandler) PathPattern() paths.Path {
	return paths.ISISDynamicHostname
}

func (h *ISISDynamicHostnameHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ISISEnabled}
}

func (h *ISISDynamicHostnameHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}

func (h *ISISDynamicHostnameHandler) Summary() string {
	return "IS-IS dynamic hostname"
}

func (h *ISISDynamicHostnameHandler) Description() string {
	return "Enable or disable IS-IS dynamic hostname resolution."
}

func (h *ISISDynamicHostnameHandler) ValueType() interface{} {
	return false
}
