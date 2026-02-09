package ospf

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewOSPFLogAdjacencyChangesHandler)
}

type OSPFLogAdjacencyChangesHandler struct{}

func NewOSPFLogAdjacencyChangesHandler(deps *deps.ConfDeps) conf.Handler {
	return &OSPFLogAdjacencyChangesHandler{}
}

func (h *OSPFLogAdjacencyChangesHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	if _, ok := hctx.NewValue.(bool); !ok {
		return fmt.Errorf("expected bool, got %T", hctx.NewValue)
	}
	return nil
}

func (h *OSPFLogAdjacencyChangesHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPFLogAdjacencyChangesHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPFLogAdjacencyChangesHandler) PathPattern() paths.Path {
	return paths.OSPFLogAdjacencyChanges
}

func (h *OSPFLogAdjacencyChangesHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.OSPFEnabled}
}

func (h *OSPFLogAdjacencyChangesHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
