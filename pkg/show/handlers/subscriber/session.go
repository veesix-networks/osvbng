package subscriber

import (
	"context"

	subscriberComp "github.com/veesix-networks/osvbng/internal/subscriber"
	"github.com/veesix-networks/osvbng/pkg/show/handlers"
	"github.com/veesix-networks/osvbng/pkg/show/paths"
)

func init() {
	handlers.RegisterFactory(NewSessionHandler)
}

type SessionHandler struct {
	subscriber *subscriberComp.Component
}

func NewSessionHandler(deps *handlers.ShowDeps) handlers.ShowHandler {
	return &SessionHandler{
		subscriber: deps.Subscriber,
	}
}

func (h *SessionHandler) Collect(ctx context.Context, req *handlers.ShowRequest) (interface{}, error) {
	sessionID := req.Options["session_id"]
	return h.subscriber.GetSession(ctx, sessionID)
}

func (h *SessionHandler) PathPattern() paths.Path {
	return paths.SubscriberSession
}

func (h *SessionHandler) Dependencies() []paths.Path {
	return nil
}
