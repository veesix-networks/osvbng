package system

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

type EventsHandler struct {
	eventBus events.Bus
}

func init() {
	show.RegisterFactory(func(deps *deps.ShowDeps) show.ShowHandler {
		return &EventsHandler{eventBus: deps.EventBus}
	})
}

func (h *EventsHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.eventBus == nil {
		return events.Stats{}, nil
	}
	return h.eventBus.Stats(), nil
}

func (h *EventsHandler) PathPattern() paths.Path {
	return paths.SystemEvents
}

func (h *EventsHandler) Dependencies() []paths.Path {
	return nil
}

func (h *EventsHandler) OutputType() interface{} {
	return &events.Stats{}
}

var _ show.ShowHandler = (*EventsHandler)(nil)
