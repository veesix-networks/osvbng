package ospf

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewOSPFMaximumPathsHandler)
}

type OSPFMaximumPathsHandler struct{}

func NewOSPFMaximumPathsHandler(deps *deps.ConfDeps) conf.Handler {
	return &OSPFMaximumPathsHandler{}
}

func (h *OSPFMaximumPathsHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	_, err := conf.ParseUint32(hctx.NewValue)
	if err != nil {
		return fmt.Errorf("invalid maximum-paths: %w", err)
	}
	return nil
}

func (h *OSPFMaximumPathsHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPFMaximumPathsHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPFMaximumPathsHandler) PathPattern() paths.Path {
	return paths.OSPFMaximumPaths
}

func (h *OSPFMaximumPathsHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.OSPFEnabled}
}

func (h *OSPFMaximumPathsHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
