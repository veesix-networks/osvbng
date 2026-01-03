package conf

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/conf/handlers"
	"github.com/veesix-networks/osvbng/pkg/conf/paths"
)

func init() {
	handlers.RegisterFactory(NewListenAddressHandler)
}

type ListenAddressHandler struct{}

func NewListenAddressHandler(deps *handlers.ConfDeps) handlers.Handler {
	return &ListenAddressHandler{}
}

func (h *ListenAddressHandler) PathPattern() paths.Path {
	return paths.Path("exporters.prometheus.listen_address")
}

func (h *ListenAddressHandler) Validate(ctx context.Context, hctx *handlers.HandlerContext) error {
	return nil
}

func (h *ListenAddressHandler) Apply(ctx context.Context, hctx *handlers.HandlerContext) error {
	return nil
}

func (h *ListenAddressHandler) Rollback(ctx context.Context, hctx *handlers.HandlerContext) error {
	return nil
}

func (h *ListenAddressHandler) Dependencies() []paths.Path {
	return nil
}

func (h *ListenAddressHandler) Callbacks() *handlers.Callbacks {
	return nil
}
