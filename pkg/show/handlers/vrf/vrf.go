package ip

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/show/handlers"
	"github.com/veesix-networks/osvbng/pkg/show/paths"
)

type VRFHandler struct {
	daemons *handlers.ShowDeps
}

func init() {
	handlers.RegisterFactory(func(daemons *handlers.ShowDeps) handlers.ShowHandler {
		return &VRFHandler{daemons: daemons}
	})
}

func (h *VRFHandler) PathPattern() paths.Path {
	return paths.VRFS
}

func (h *VRFHandler) Dependencies() []paths.Path {
	return nil
}

func (h *VRFHandler) Collect(ctx context.Context, req *handlers.ShowRequest) (interface{}, error) {
	return nil, fmt.Errorf("Router component not yet implemented")
}
