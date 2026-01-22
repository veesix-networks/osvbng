package auth

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/provider"
)

type AuthProvider interface {
	provider.Provider
	Authenticate(ctx context.Context, req *AuthRequest) (*AuthResponse, error)
	StartAccounting(ctx context.Context, session *Session) error
	UpdateAccounting(ctx context.Context, session *Session) error
	StopAccounting(ctx context.Context, session *Session) error
}

type AuthRequest struct {
	Username      string
	MAC           string
	AcctSessionID string
	Attributes    map[string]string

	SVLAN     uint16
	CVLAN     uint16
	Interface string

	SubscriberGroup *subscriber.SubscriberGroup
}

type AuthResponse struct {
	Allowed    bool
	Attributes map[string]string
}

type Session struct {
	SessionID       string
	AcctSessionID   string
	Username        string
	MAC             string
	RxPackets       uint64
	RxBytes         uint64
	TxPackets       uint64
	TxBytes         uint64
	SessionDuration uint32
	Attributes      map[string]string
}
