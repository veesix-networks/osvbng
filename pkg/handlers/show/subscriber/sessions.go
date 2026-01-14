package subscriber

import (
	"context"
	"strconv"

	subscriberComp "github.com/veesix-networks/osvbng/internal/subscriber"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/state"
	statepaths "github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	show.RegisterFactory(NewSessionsHandler)

	state.RegisterMetric(statepaths.SubscriberSessions, paths.SubscriberSessions)
}

type SessionsHandler struct {
	subscriber *subscriberComp.Component
}

func NewSessionsHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &SessionsHandler{
		subscriber: deps.Subscriber,
	}
}

func (h *SessionsHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
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
