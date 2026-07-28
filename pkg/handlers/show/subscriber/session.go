package subscriber

import (
	"context"
	"fmt"

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
	username := req.Options["username"]

	if sessionID != "" {
		return h.subscriber.GetSession(ctx, sessionID)
	}

	if username != "" {
		sessions, err := h.subscriber.GetSessions(ctx, "", "", 0, username)
		if err != nil {
			return nil, err
		}
		if len(sessions) == 0 {
			return nil, fmt.Errorf("session not found for username: %s", username)
		}
		return sessions[0], nil
	}

	return nil, fmt.Errorf("either --session-id or --username must be specified")
}

func (h *SessionHandler) PathPattern() paths.Path {
	return paths.SubscriberSession
}

func (h *SessionHandler) Dependencies() []paths.Path {
	return nil
}

func (h *SessionHandler) Summary() string {
	return "Show a single subscriber session"
}

func (h *SessionHandler) Description() string {
	return "Retrieve a specific subscriber session by session ID or username."
}

type SessionOptions struct {
	SessionID string `query:"session_id" description:"Subscriber session identifier"`
	Username  string `query:"username" description:"Filter by username"`
}

func (h *SessionHandler) OptionsType() interface{} {
	return &SessionOptions{}
}
