package ospf6

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewOSPF6LogAdjacencyChangesHandler)
}

type OSPF6LogAdjacencyChangesHandler struct{}

func NewOSPF6LogAdjacencyChangesHandler(deps *deps.ConfDeps) conf.Handler {
	return &OSPF6LogAdjacencyChangesHandler{}
}

func (h *OSPF6LogAdjacencyChangesHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	if _, ok := hctx.NewValue.(bool); !ok {
		return fmt.Errorf("expected bool, got %T", hctx.NewValue)
	}
	return nil
}

func (h *OSPF6LogAdjacencyChangesHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPF6LogAdjacencyChangesHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPF6LogAdjacencyChangesHandler) PathPattern() paths.Path {
	return paths.OSPF6LogAdjacencyChanges
}

func (h *OSPF6LogAdjacencyChangesHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.OSPF6Enabled}
}

func (h *OSPF6LogAdjacencyChangesHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
