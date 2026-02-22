package aaa

import (
	"context"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/veesix-networks/osvbng/pkg/auth"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/provider"
)

type noopAuthProvider struct{}

func (noopAuthProvider) Info() provider.Info                                           { return provider.Info{} }
func (noopAuthProvider) Authenticate(context.Context, *auth.AuthRequest) (*auth.AuthResponse, error) {
	return &auth.AuthResponse{}, nil
}
func (noopAuthProvider) StartAccounting(context.Context, *auth.Session) error  { return nil }
func (noopAuthProvider) UpdateAccounting(context.Context, *auth.Session) error { return nil }
func (noopAuthProvider) StopAccounting(context.Context, *auth.Session) error   { return nil }

func TestCalculateBucket(t *testing.T) {
	tests := []struct {
		name       string
		authTime   time.Time
		bucketSize time.Duration
		want       int
	}{
		{
			name:       "bucket 0 - first second",
			authTime:   time.Date(2025, 12, 12, 12, 0, 0, 230000000, time.UTC),
			bucketSize: 5 * time.Second,
			want:       0,
		},
		{
			name:       "bucket 0 - near end",
			authTime:   time.Date(2025, 12, 12, 12, 0, 4, 990000000, time.UTC),
			bucketSize: 5 * time.Second,
			want:       0,
		},
		{
			name:       "bucket 1",
			authTime:   time.Date(2025, 12, 12, 12, 0, 7, 440000000, time.UTC),
			bucketSize: 5 * time.Second,
			want:       1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateBucket(tt.authTime, tt.bucketSize)
			if got != tt.want {
				t.Errorf("calculateBucket() = %v, want %v", got, tt.want)
			}
		})
	}
}

type testSession struct {
	sessionID string
	authTime  time.Time
	session   *models.IPoESession
}

func createTestSession(sessionID string, authTime time.Time, macStr string, ipAddr string) testSession {
	mac, _ := net.ParseMAC(macStr)
	ip := net.ParseIP(ipAddr)

	return testSession{
		sessionID: sessionID,
		authTime:  authTime,
		session: &models.IPoESession{
			SessionID:   sessionID,
			State:       models.SessionStateActive,
			MAC:         mac,
			OuterVLAN:   100,
			InnerVLAN:   0,
			VLANCount:   1,
			IPv4Address: ip,
			LeaseTime:   3600,
		},
	}
}

func TestHandleSessionLifecycle(t *testing.T) {
	base := component.NewBase("aaa-test")
	base.StartContext(context.Background())
	defer base.StopContext()

	c := &Component{
		Base:         base,
		logger:       slog.Default(),
		authProvider: noopAuthProvider{},
		buckets:      make(map[int][]string),
		acctCache:    make(map[string]*AccountingSession),
	}

	tests := []struct {
		name       string
		session    testSession
		wantBucket int
	}{
		{
			name:       "session auth at 0.5s goes to bucket 0",
			session:    createTestSession("8a179135-4a4d-4ac0-89ba-827618ebbf57", time.Date(2025, 12, 13, 10, 23, 0, 500000000, time.UTC), "02:00:00:00:00:01", "10.1.0.163"),
			wantBucket: 0,
		},
		{
			name:       "session auth at 3.4s goes to bucket 0",
			session:    createTestSession("7b289246-5b5e-4bd1-9acb-938729fccg68", time.Date(2025, 12, 13, 10, 23, 3, 440000000, time.UTC), "02:00:00:00:00:02", "10.1.0.164"),
			wantBucket: 0,
		},
		{
			name:       "session auth at 7.8s goes to bucket 1",
			session:    createTestSession("9c390357-6c6f-5ce2-abdc-a49830gddh79", time.Date(2025, 12, 13, 10, 23, 7, 800000000, time.UTC), "02:00:00:00:00:03", "10.1.0.165"),
			wantBucket: 1,
		},
		{
			name:       "session auth at 59.9s goes to bucket 11",
			session:    createTestSession("ad4a1468-7d7g-6df3-bced-b5a941heei8a", time.Date(2025, 12, 13, 10, 23, 59, 900000000, time.UTC), "02:00:00:00:00:04", "10.1.0.166"),
			wantBucket: 11,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := events.Event{
				Timestamp: tt.session.authTime,
				Data: &events.SessionLifecycleEvent{
					AccessType: models.AccessTypeIPoE,
					Protocol:   models.ProtocolDHCPv4,
					SessionID:  tt.session.sessionID,
					State:      tt.session.session.State,
					Session:    tt.session.session,
				},
			}

			c.handleSessionLifecycle(event)

			sessions := c.buckets[tt.wantBucket]
			if len(sessions) == 0 {
				t.Fatalf("expected session in bucket %d, but bucket is empty", tt.wantBucket)
			}

			found := false
			for _, sid := range sessions {
				if sid == tt.session.sessionID {
					found = true
					break
				}
			}

			if !found {
				t.Errorf("session %s not found in bucket %d", tt.session.sessionID, tt.wantBucket)
			}
		})
	}
	t.Logf("Buckets: %+v", c.buckets)
}
