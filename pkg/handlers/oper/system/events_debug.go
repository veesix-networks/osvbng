package system

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper/paths"
)

func init() {
	oper.RegisterFactory(NewEventsDebugHandler)
}

type EventsDebugHandler struct {
	deps *deps.OperDeps
}

type EventsDebugRequest struct {
	Topics []string `json:"topics"`
}

type EventsDebugResponse struct {
	Topics []string `json:"topics"`
}

func NewEventsDebugHandler(deps *deps.OperDeps) oper.OperHandler {
	return &EventsDebugHandler{deps: deps}
}

func (h *EventsDebugHandler) Execute(ctx context.Context, req *oper.Request) (interface{}, error) {
	if h.deps.EventBus == nil {
		return nil, fmt.Errorf("event bus not available")
	}

	var body EventsDebugRequest
	if err := json.Unmarshal(req.Body, &body); err != nil {
		return nil, fmt.Errorf("invalid request body: %w", err)
	}

	h.deps.EventBus.SetDebugTopics(body.Topics)

	return &EventsDebugResponse{
		Topics: h.deps.EventBus.DebugTopics(),
	}, nil
}

func (h *EventsDebugHandler) PathPattern() paths.Path {
	return paths.SystemEventsDebug
}

func (h *EventsDebugHandler) Dependencies() []paths.Path {
	return nil
}

func (h *EventsDebugHandler) InputType() interface{} {
	return &EventsDebugRequest{}
}

func (h *EventsDebugHandler) OutputType() interface{} {
	return &EventsDebugResponse{}
}
