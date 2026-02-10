package subscriber

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper/paths"
)

func init() {
	oper.RegisterFactory(NewClearSessionHandler)
}

type ClearSessionHandler struct {
	deps *deps.OperDeps
}

type ClearSessionRequest struct {
	SessionID string `json:"session_id,omitempty"`
	MAC       string `json:"mac,omitempty"`
	IPv4      string `json:"ipv4,omitempty"`
	IPv6      string `json:"ipv6,omitempty"`
	Username  string `json:"username,omitempty"`
}

type ClearSessionResponse struct {
	Terminated int      `json:"terminated"`
	Sessions   []string `json:"sessions"`
}

func NewClearSessionHandler(deps *deps.OperDeps) oper.OperHandler {
	return &ClearSessionHandler{deps: deps}
}

func (h *ClearSessionHandler) Execute(ctx context.Context, req *oper.Request) (interface{}, error) {
	if h.deps.Subscriber == nil {
		return nil, fmt.Errorf("subscriber component not available")
	}

	var body ClearSessionRequest
	if err := json.Unmarshal(req.Body, &body); err != nil {
		return nil, fmt.Errorf("invalid request body: %w", err)
	}

	if body.SessionID == "" && body.MAC == "" && body.IPv4 == "" && body.IPv6 == "" && body.Username == "" {
		return nil, fmt.Errorf("at least one filter must be specified")
	}

	sessions, err := h.deps.Subscriber.GetSessions(ctx, "", "", 0)
	if err != nil {
		return nil, fmt.Errorf("get sessions: %w", err)
	}

	var terminated []string

	for _, sess := range sessions {
		if !h.matches(sess.GetSessionID(), sess.GetMAC(), sess.GetIPv4Address(), sess.GetIPv6Address(), sess.GetUsername(), &body) {
			continue
		}

		if err := h.deps.Subscriber.TerminateSession(ctx, sess.GetSessionID()); err != nil {
			return nil, fmt.Errorf("terminate session %s: %w", sess.GetSessionID(), err)
		}

		terminated = append(terminated, sess.GetSessionID())
	}

	return &ClearSessionResponse{
		Terminated: len(terminated),
		Sessions:   terminated,
	}, nil
}

func (h *ClearSessionHandler) matches(sessionID string, mac net.HardwareAddr, ipv4, ipv6 net.IP, username string, filter *ClearSessionRequest) bool {
	if filter.SessionID != "" && filter.SessionID != sessionID {
		return false
	}

	if filter.MAC != "" {
		if mac == nil || !strings.EqualFold(mac.String(), strings.ToLower(filter.MAC)) {
			return false
		}
	}

	if filter.IPv4 != "" {
		filterIP := net.ParseIP(filter.IPv4)
		if filterIP == nil || ipv4 == nil || !ipv4.Equal(filterIP) {
			return false
		}
	}

	if filter.IPv6 != "" {
		filterIP := net.ParseIP(filter.IPv6)
		if filterIP == nil || ipv6 == nil || !ipv6.Equal(filterIP) {
			return false
		}
	}

	if filter.Username != "" && filter.Username != username {
		return false
	}

	return true
}

func (h *ClearSessionHandler) PathPattern() paths.Path {
	return paths.SubscriberSessionClear
}

func (h *ClearSessionHandler) Dependencies() []paths.Path {
	return nil
}
