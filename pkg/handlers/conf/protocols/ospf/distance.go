package ospf

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewOSPFDistanceHandler)
}

type OSPFDistanceHandler struct{}

func NewOSPFDistanceHandler(deps *deps.ConfDeps) conf.Handler {
	return &OSPFDistanceHandler{}
}

func (h *OSPFDistanceHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	val, err := conf.ParseUint32(hctx.NewValue)
	if err != nil {
		return fmt.Errorf("invalid distance: %w", err)
	}

	if val < 1 || val > 255 {
		return fmt.Errorf("distance %d out of range (1-255)", val)
	}

	return nil
}

func (h *OSPFDistanceHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPFDistanceHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPFDistanceHandler) PathPattern() paths.Path {
	return paths.OSPFDistance
}

func (h *OSPFDistanceHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.OSPFEnabled}
}

func (h *OSPFDistanceHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
