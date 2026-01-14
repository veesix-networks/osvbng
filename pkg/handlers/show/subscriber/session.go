package subscriber

import (
	"context"

	subscriberComp "github.com/veesix-networks/osvbng/internal/subscriber"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(NewSessionHandler)
}

type SessionHandler struct {
	subscriber *subscriberComp.Component
}

func NewSessionHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &SessionHandler{
		subscriber: deps.Subscriber,
	}
}

func (h *SessionHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	sessionID := req.Options["session_id"]
	return h.subscriber.GetSession(ctx, sessionID)
}

func (h *SessionHandler) PathPattern() paths.Path {
	return paths.SubscriberSession
}

func (h *SessionHandler) Dependencies() []paths.Path {
	return nil
}
