package vrfs

import (
	"github.com/veesix-networks/osvbng/pkg/deps"
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/types"
	"github.com/veesix-networks/osvbng/pkg/models/vrf"
	"github.com/veesix-networks/osvbng/pkg/operations"
)

func init() {
	conf.RegisterFactory(NewVRFHandler)
}

type VRFHandler struct {
	dataplane operations.Dataplane
}

func NewVRFHandler(daemons *deps.ConfDeps) conf.Handler {
	return &VRFHandler{dataplane: daemons.Dataplane}
}

func (h *VRFHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg, ok := hctx.NewValue.(*types.VRFSConfig)
	if !ok {
		return fmt.Errorf("expected *types.VRFSConfig, got %T", hctx.NewValue)
	}

	if err := vrf.ValidateVRFName(cfg.Name); err != nil {
		return err
	}

	return nil
}

func (h *VRFHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *VRFHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *VRFHandler) PathPattern() paths.Path {
	return paths.VRFS
}

func (h *VRFHandler) Dependencies() []paths.Path {
	return nil
}

func (h *VRFHandler) Callbacks() *conf.Callbacks {
	return nil
}
