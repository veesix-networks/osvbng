package conf

import (
	"github.com/veesix-networks/osvbng/pkg/deps"
	"context"

	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewListenAddressHandler)
}

type ListenAddressHandler struct{}

func NewListenAddressHandler(deps *deps.ConfDeps) conf.Handler {
	return &ListenAddressHandler{}
}

func (h *ListenAddressHandler) PathPattern() paths.Path {
	return paths.Path("exporters.prometheus.listen_address")
}

func (h *ListenAddressHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ListenAddressHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ListenAddressHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ListenAddressHandler) Dependencies() []paths.Path {
	return nil
}

func (h *ListenAddressHandler) Callbacks() *conf.Callbacks {
	return nil
}
