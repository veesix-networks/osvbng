package mpls

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewPlatformLabelsHandler)
}

type PlatformLabelsHandler struct{}

func NewPlatformLabelsHandler(deps *deps.ConfDeps) conf.Handler {
	return &PlatformLabelsHandler{}
}

func (h *PlatformLabelsHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	v, ok := hctx.NewValue.(uint32)
	if !ok {
		return fmt.Errorf("expected uint32, got %T", hctx.NewValue)
	}
	if v < 16 {
		return fmt.Errorf("platform-labels must be >= 16, got %d", v)
	}
	return nil
}

func (h *PlatformLabelsHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	v, _ := hctx.NewValue.(uint32)
	setPlatformLabels(v)
	return nil
}

func (h *PlatformLabelsHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *PlatformLabelsHandler) PathPattern() paths.Path {
	return paths.MPLSPlatformLabels
}

func (h *PlatformLabelsHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.MPLSEnabled}
}

func (h *PlatformLabelsHandler) Callbacks() *conf.Callbacks {
	return nil
}
