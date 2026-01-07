package vrfs

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/conf/handlers"
	"github.com/veesix-networks/osvbng/pkg/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/conf/types"
	"github.com/veesix-networks/osvbng/pkg/models/vrf"
	"github.com/veesix-networks/osvbng/pkg/operations"
)

func init() {
	handlers.RegisterFactory(NewVRFHandler)
}

type VRFHandler struct {
	dataplane operations.Dataplane
}

func NewVRFHandler(daemons *handlers.ConfDeps) handlers.Handler {
	return &VRFHandler{dataplane: daemons.Dataplane}
}

func (h *VRFHandler) Validate(ctx context.Context, hctx *handlers.HandlerContext) error {
	cfg, ok := hctx.NewValue.(*types.VRFSConfig)
	if !ok {
		return fmt.Errorf("expected *types.VRFSConfig, got %T", hctx.NewValue)
	}

	if err := vrf.ValidateVRFName(cfg.Name); err != nil {
		return err
	}

	return nil
}

func (h *VRFHandler) Apply(ctx context.Context, hctx *handlers.HandlerContext) error {
	return nil
}

func (h *VRFHandler) Rollback(ctx context.Context, hctx *handlers.HandlerContext) error {
	return nil
}

func (h *VRFHandler) PathPattern() paths.Path {
	return paths.VRFS
}

func (h *VRFHandler) Dependencies() []paths.Path {
	return nil
}

func (h *VRFHandler) Callbacks() *handlers.Callbacks {
	return nil
}
