package aaa

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/show/handlers"
	"github.com/veesix-networks/osvbng/pkg/show/paths"
	"github.com/veesix-networks/osvbng/pkg/state"
	statepaths "github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	handlers.RegisterFactory(NewRADIUSServersHandler)

	state.RegisterMetric(statepaths.AAARadiusServers, paths.AAARadiusServers)
}

type RADIUSServersHandler struct {
}

func NewRADIUSServersHandler(deps *handlers.ShowDeps) handlers.ShowHandler {
	return &RADIUSServersHandler{}
}

func (h *RADIUSServersHandler) Collect(ctx context.Context, req *handlers.ShowRequest) (interface{}, error) {
	return nil, fmt.Errorf("AAA component not yet implemented")
}


func (h *RADIUSServersHandler) PathPattern() paths.Path {
	return paths.AAARadiusServers
}

func (h *RADIUSServersHandler) Dependencies() []paths.Path {
	return nil
}
