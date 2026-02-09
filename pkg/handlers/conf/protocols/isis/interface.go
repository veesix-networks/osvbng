package isis

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/protocols"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewISISInterfaceHandler)
}

type ISISInterfaceHandler struct{}

func NewISISInterfaceHandler(deps *deps.ConfDeps) conf.Handler {
	return &ISISInterfaceHandler{}
}

func (h *ISISInterfaceHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg, ok := hctx.NewValue.(*protocols.ISISInterfaceConfig)
	if !ok {
		return fmt.Errorf("expected *protocols.ISISInterfaceConfig, got %T", hctx.NewValue)
	}

	if cfg == nil {
		return nil
	}

	if cfg.Network != "" && cfg.Network != "point-to-point" {
		return fmt.Errorf("invalid ISIS network type %q: must be point-to-point", cfg.Network)
	}

	if cfg.CircuitType != "" && !cfg.CircuitType.Valid() {
		return fmt.Errorf("invalid circuit-type %q", cfg.CircuitType)
	}

	if cfg.HelloInterval != 0 && cfg.HelloInterval > 65535 {
		return fmt.Errorf("hello-interval %d out of range (1-65535)", cfg.HelloInterval)
	}

	if cfg.HelloMultiplier != 0 && cfg.HelloMultiplier > 65535 {
		return fmt.Errorf("hello-multiplier %d out of range (1-65535)", cfg.HelloMultiplier)
	}

	return nil
}

func (h *ISISInterfaceHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISInterfaceHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISInterfaceHandler) PathPattern() paths.Path {
	return paths.ISISInterface
}

func (h *ISISInterfaceHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ISISEnabled}
}

func (h *ISISInterfaceHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
