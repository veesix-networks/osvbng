package ospf6

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewOSPF6MaximumPathsHandler)
}

type OSPF6MaximumPathsHandler struct{}

func NewOSPF6MaximumPathsHandler(deps *deps.ConfDeps) conf.Handler {
	return &OSPF6MaximumPathsHandler{}
}

func (h *OSPF6MaximumPathsHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	_, err := conf.ParseUint32(hctx.NewValue)
	if err != nil {
		return fmt.Errorf("invalid maximum-paths: %w", err)
	}
	return nil
}

func (h *OSPF6MaximumPathsHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPF6MaximumPathsHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPF6MaximumPathsHandler) PathPattern() paths.Path {
	return paths.OSPF6MaximumPaths
}

func (h *OSPF6MaximumPathsHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.OSPF6Enabled}
}

func (h *OSPF6MaximumPathsHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
