package subscriber

import (
	"context"
	"strconv"

	subscriberComp "github.com/veesix-networks/osvbng/internal/subscriber"
	"github.com/veesix-networks/osvbng/pkg/show/handlers"
	"github.com/veesix-networks/osvbng/pkg/show/paths"
)

func init() {
	handlers.RegisterFactory(NewSessionsHandler)
}

type SessionsHandler struct {
	subscriber *subscriberComp.Component
}

func NewSessionsHandler(deps *handlers.ShowDeps) handlers.ShowHandler {
	return &SessionsHandler{
		subscriber: deps.Subscriber,
	}
}

func (h *SessionsHandler) Collect(ctx context.Context, req *handlers.ShowRequest) (interface{}, error) {
	accessType := req.Options["access_type"]
	protocol := req.Options["protocol"]
	svlanStr := req.Options["svlan"]

	var svlan uint32
	if svlanStr != "" {
		val, _ := strconv.ParseUint(svlanStr, 10, 32)
		svlan = uint32(val)
	}

	return h.subscriber.GetSessions(ctx, accessType, protocol, svlan)
}

func (h *SessionsHandler) PathPattern() paths.Path {
	return paths.SubscriberSessions
}

func (h *SessionsHandler) Dependencies() []paths.Path {
	return nil
}
