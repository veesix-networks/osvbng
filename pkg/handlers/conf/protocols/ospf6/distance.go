package ospf6

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewOSPF6DistanceHandler)
}

type OSPF6DistanceHandler struct{}

func NewOSPF6DistanceHandler(deps *deps.ConfDeps) conf.Handler {
	return &OSPF6DistanceHandler{}
}

func (h *OSPF6DistanceHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	val, err := conf.ParseUint32(hctx.NewValue)
	if err != nil {
		return fmt.Errorf("invalid distance: %w", err)
	}

	if val < 1 || val > 255 {
		return fmt.Errorf("distance %d out of range (1-255)", val)
	}

	return nil
}

func (h *OSPF6DistanceHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPF6DistanceHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPF6DistanceHandler) PathPattern() paths.Path {
	return paths.OSPF6Distance
}

func (h *OSPF6DistanceHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.OSPF6Enabled}
}

func (h *OSPF6DistanceHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
