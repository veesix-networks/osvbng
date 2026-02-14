package ip

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/vrfmgr"
)

type VRFHandler struct {
	vrfMgr *vrfmgr.Manager
}

func init() {
	show.RegisterFactory(func(daemons *deps.ShowDeps) show.ShowHandler {
		return &VRFHandler{vrfMgr: daemons.VRFManager}
	})
}

func (h *VRFHandler) PathPattern() paths.Path {
	return paths.VRFS
}

func (h *VRFHandler) Dependencies() []paths.Path {
	return nil
}

func (h *VRFHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	return h.vrfMgr.GetVRFs(), nil
}
