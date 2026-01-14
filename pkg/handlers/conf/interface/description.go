package iface

import (
	"github.com/veesix-networks/osvbng/pkg/deps"
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/operations"
)

func init() {
	conf.RegisterFactory(NewDescriptionHandler)
}

type DescriptionHandler struct {
	dataplane operations.Dataplane
}

func NewDescriptionHandler(daemons *deps.ConfDeps) conf.Handler {
	return &DescriptionHandler{dataplane: daemons.Dataplane}
}

func (h *DescriptionHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	desc, ok := hctx.NewValue.(string)
	if !ok {
		return fmt.Errorf("description must be a string")
	}

	if len(desc) > 255 {
		return fmt.Errorf("description too long (max 255 characters)")
	}

	return nil
}

func (h *DescriptionHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	ifName := conf.ExtractInterfaceName(hctx.Path)
	desc := hctx.NewValue.(string)

	return h.dataplane.SetInterfaceDescription(ifName, desc)
}

func (h *DescriptionHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	ifName := conf.ExtractInterfaceName(hctx.Path)
	oldDesc := ""
	if hctx.OldValue != nil {
		oldDesc = hctx.OldValue.(string)
	}

	return h.dataplane.SetInterfaceDescription(ifName, oldDesc)
}

func (h *DescriptionHandler) PathPattern() paths.Path {
	return paths.InterfaceDescription
}

func (h *DescriptionHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.Interface}
}

func (h *DescriptionHandler) Callbacks() *conf.Callbacks {
	return nil
}
