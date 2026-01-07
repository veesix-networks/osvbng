package conf

import (
	"github.com/veesix-networks/osvbng/pkg/deps"
	"context"

	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewEnabledHandler)
}

type EnabledHandler struct{}

func NewEnabledHandler(deps *deps.ConfDeps) conf.Handler {
	return &EnabledHandler{}
}

func (h *EnabledHandler) PathPattern() paths.Path {
	return paths.Path("exporters.prometheus.enabled")
}

func (h *EnabledHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *EnabledHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *EnabledHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *EnabledHandler) Dependencies() []paths.Path {
	return nil
}

func (h *EnabledHandler) Callbacks() *conf.Callbacks {
	return nil
}
