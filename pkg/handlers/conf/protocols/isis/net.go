package isis

import (
	"context"
	"fmt"
	"regexp"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

var netRegex = regexp.MustCompile(`^[0-9a-fA-F]{2}(\.[0-9a-fA-F]{4}){3,9}\.[0-9a-fA-F]{2}$`)

func init() {
	conf.RegisterFactory(NewISISNETHandler)
}

type ISISNETHandler struct{}

func NewISISNETHandler(deps *deps.ConfDeps) conf.Handler {
	return &ISISNETHandler{}
}

func (h *ISISNETHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	val, ok := hctx.NewValue.(string)
	if !ok {
		return fmt.Errorf("expected string, got %T", hctx.NewValue)
	}

	if val != "" && !netRegex.MatchString(val) {
		return fmt.Errorf("invalid NET %q: must be ISO format XX.XXXX.XXXX.XXXX.XX", val)
	}

	return nil
}

func (h *ISISNETHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISNETHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISNETHandler) PathPattern() paths.Path {
	return paths.ISISNET
}

func (h *ISISNETHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ISISEnabled}
}

func (h *ISISNETHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
