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
	conf.RegisterFactory(NewISISIsTypeHandler)
}

type ISISIsTypeHandler struct{}

func NewISISIsTypeHandler(deps *deps.ConfDeps) conf.Handler {
	return &ISISIsTypeHandler{}
}

func (h *ISISIsTypeHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	val, ok := hctx.NewValue.(protocols.ISISIsType)
	if !ok {
		return fmt.Errorf("expected protocols.ISISIsType, got %T", hctx.NewValue)
	}

	if val != "" && !val.Valid() {
		return fmt.Errorf("invalid is-type %q", val)
	}

	return nil
}

func (h *ISISIsTypeHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISIsTypeHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISIsTypeHandler) PathPattern() paths.Path {
	return paths.ISISIsType
}

func (h *ISISIsTypeHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ISISEnabled}
}

func (h *ISISIsTypeHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}

func (h *ISISIsTypeHandler) Summary() string {
	return "IS-IS router type"
}

func (h *ISISIsTypeHandler) Description() string {
	return "Set the IS-IS router type to level-1, level-2, or level-1-2."
}

func (h *ISISIsTypeHandler) ValueType() interface{} {
	return ""
}
