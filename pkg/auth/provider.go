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

	SVLAN           uint16
	CVLAN           uint16
	Interface       string
	AccessIfIndex   uint32
	AccessInterface string
	AccessType      string
	PolicyName      string

	// UsernameFallback indicates Username is the MAC fallback because the
	// policy.format could not be resolved (a referenced identity token was
	// absent). RADIUS rejects on this so a configured identity is never
	// silently replaced by the MAC; local authorises normally.
	UsernameFallback bool

	SubscriberGroup *subscriber.SubscriberGroup
}

type AuthResponse struct {
	Allowed    bool
	Attributes map[string]string
}

type Session struct {
	SessionID         string
	AcctSessionID     string
	Username          string
	MAC               string
	AccessType        string
	AccessInterface   string
	SVLAN             uint16
	CVLAN             uint16
	AccessIfIndex     uint32
	SubscriberIfIndex uint32
	RxPackets         uint64
	RxBytes           uint64
	TxPackets         uint64
	TxBytes           uint64
	SessionDuration   uint32
	Attributes        map[string]string
}
