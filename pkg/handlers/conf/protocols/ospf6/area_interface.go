package ospf6

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/protocols"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewOSPF6AreaInterfaceHandler)
}

type OSPF6AreaInterfaceHandler struct{}

func NewOSPF6AreaInterfaceHandler(deps *deps.ConfDeps) conf.Handler {
	return &OSPF6AreaInterfaceHandler{}
}

func (h *OSPF6AreaInterfaceHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg, ok := hctx.NewValue.(*protocols.OSPF6InterfaceConfig)
	if !ok {
		return fmt.Errorf("expected *protocols.OSPF6InterfaceConfig, got %T", hctx.NewValue)
	}

	if cfg == nil {
		return nil
	}

	if cfg.Network != "" && !cfg.Network.Valid() {
		return fmt.Errorf("invalid OSPFv3 network type %q", cfg.Network)
	}

	if cfg.HelloInterval != 0 && (cfg.HelloInterval < 1 || cfg.HelloInterval > 65535) {
		return fmt.Errorf("hello-interval %d out of range (1-65535)", cfg.HelloInterval)
	}

	if cfg.DeadInterval != 0 && (cfg.DeadInterval < 1 || cfg.DeadInterval > 65535) {
		return fmt.Errorf("dead-interval %d out of range (1-65535)", cfg.DeadInterval)
	}

	return nil
}

func (h *OSPF6AreaInterfaceHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPF6AreaInterfaceHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPF6AreaInterfaceHandler) PathPattern() paths.Path {
	return paths.OSPF6AreaInterface
}

func (h *OSPF6AreaInterfaceHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.OSPF6Enabled}
}

func (h *OSPF6AreaInterfaceHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
