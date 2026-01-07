package aaa

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/state"
	statepaths "github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	show.RegisterFactory(NewRADIUSServersHandler)

	state.RegisterMetric(statepaths.AAARadiusServers, paths.AAARadiusServers)
}

type RADIUSServersHandler struct {
}

func NewRADIUSServersHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &RADIUSServersHandler{}
}

func (h *RADIUSServersHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	return nil, fmt.Errorf("AAA component not yet implemented")
}


func (h *RADIUSServersHandler) PathPattern() paths.Path {
	return paths.AAARadiusServers
}

func (h *RADIUSServersHandler) Dependencies() []paths.Path {
	return nil
}
