package isis

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewISISLogAdjacencyChangesHandler)
}

type ISISLogAdjacencyChangesHandler struct{}

func NewISISLogAdjacencyChangesHandler(deps *deps.ConfDeps) conf.Handler {
	return &ISISLogAdjacencyChangesHandler{}
}

func (h *ISISLogAdjacencyChangesHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	if _, ok := hctx.NewValue.(bool); !ok {
		return fmt.Errorf("expected bool, got %T", hctx.NewValue)
	}
	return nil
}

func (h *ISISLogAdjacencyChangesHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISLogAdjacencyChangesHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISLogAdjacencyChangesHandler) PathPattern() paths.Path {
	return paths.ISISLogAdjacencyChanges
}

func (h *ISISLogAdjacencyChangesHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ISISEnabled}
}

func (h *ISISLogAdjacencyChangesHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
