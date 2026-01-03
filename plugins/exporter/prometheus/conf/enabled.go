package conf

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/conf/handlers"
	"github.com/veesix-networks/osvbng/pkg/conf/paths"
)

func init() {
	handlers.RegisterFactory(NewEnabledHandler)
}

type EnabledHandler struct{}

func NewEnabledHandler(deps *handlers.ConfDeps) handlers.Handler {
	return &EnabledHandler{}
}

func (h *EnabledHandler) PathPattern() paths.Path {
	return paths.Path("exporters.prometheus.enabled")
}

func (h *EnabledHandler) Validate(ctx context.Context, hctx *handlers.HandlerContext) error {
	return nil
}

func (h *EnabledHandler) Apply(ctx context.Context, hctx *handlers.HandlerContext) error {
	return nil
}

func (h *EnabledHandler) Rollback(ctx context.Context, hctx *handlers.HandlerContext) error {
	return nil
}

func (h *EnabledHandler) Dependencies() []paths.Path {
	return nil
}

func (h *EnabledHandler) Callbacks() *handlers.Callbacks {
	return nil
}
