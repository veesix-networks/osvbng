package ldp

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/state"
	statepaths "github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	show.RegisterFactory(NewLDPBindingsHandler)
	state.RegisterMetric(statepaths.ProtocolsLDPBindings, paths.ProtocolsLDPBindings)
}

type LDPBindingsHandler struct {
	routing *routing.Component
}

func NewLDPBindingsHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &LDPBindingsHandler{routing: deps.Routing}
}

func (h *LDPBindingsHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetLDPBindings()
}

func (h *LDPBindingsHandler) PathPattern() paths.Path {
	return paths.ProtocolsLDPBindings
}

func (h *LDPBindingsHandler) Dependencies() []paths.Path {
	return nil
}
