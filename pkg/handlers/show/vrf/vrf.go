package ip

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

type VRFHandler struct {
	daemons *deps.ShowDeps
}

func init() {
	show.RegisterFactory(func(daemons *deps.ShowDeps) show.ShowHandler {
		return &VRFHandler{daemons: daemons}
	})
}

func (h *VRFHandler) PathPattern() paths.Path {
	return paths.VRFS
}

func (h *VRFHandler) Dependencies() []paths.Path {
	return nil
}

func (h *VRFHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	return nil, fmt.Errorf("Router component not yet implemented")
}
