package aaa

import (
	"context"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/veesix-networks/osvbng/pkg/models"
)

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
	session   *models.DHCPv4Session
}

func createTestSession(sessionID string, authTime time.Time, macStr string, ipAddr string) testSession {
	mac, _ := net.ParseMAC(macStr)
	ip := net.ParseIP(ipAddr)

	return testSession{
		sessionID: sessionID,
		authTime:  authTime,
		session: &models.DHCPv4Session{
			SessionID:        sessionID,
			State:            models.SessionStateActive,
			MAC:              mac,
			OuterVLAN:        100,
			InnerVLAN:        0,
			VLANCount:        1,
			IPv4Address:      ip,
			LeaseTime:        3600,
			RADIUSAttributes: map[string]string{},
			CreatedAt:        authTime,
		},
	}
}

func TestBuildBuckets(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := &Daemon{
		authServers: []ServerConfig{
			{
				Address: "127.0.0.1:1812",
				Secret:  "testing123",
				Timeout: 3 * time.Second,
			},
		},
		acctServers: []ServerConfig{
			{
				Address: "127.0.0.1:1813",
				Secret:  "testing123",
				Timeout: 3 * time.Second,
			},
		},
		buckets:    make(map[int][]string),
		bucketSize: 1 * time.Second,
		ctx:        ctx,
		logger:     slog.Default(),
		acctCache:  make(map[string]*AccountingSession),
		stats:      NewRADIUSStats(),
	}

	now := time.Now()
	targetSecond := (now.Second() + 5) % 60

	authTime1 := now.Truncate(time.Minute).Add(time.Duration(targetSecond) * time.Second).Add(-6 * time.Minute)
	session1 := createTestSession("session-1", authTime1, "02:00:00:00:00:01", "10.1.0.1")

	authTime2 := now.Truncate(time.Minute).Add(time.Duration(targetSecond) * time.Second).Add(-11 * time.Minute)
	session2 := createTestSession("session-2", authTime2, "02:00:00:00:00:02", "10.1.0.2")

	event1 := models.Event{
		Type:       models.EventTypeSessionLifecycle,
		Timestamp:  authTime1,
		SessionID:  session1.sessionID,
		AccessType: models.AccessTypeIPoE,
		Protocol:   models.ProtocolDHCPv4,
	}
	event1.SetPayload(session1.session)

	err := d.handleSessionLifecycle(event1)
	if err != nil {
		t.Fatalf("handleSessionLifecycle failed: %v", err)
	}

	event2 := models.Event{
		Type:       models.EventTypeSessionLifecycle,
		Timestamp:  authTime2,
		SessionID:  session2.sessionID,
		AccessType: models.AccessTypeIPoE,
		Protocol:   models.ProtocolDHCPv4,
	}
	event2.SetPayload(session2.session)

	err = d.handleSessionLifecycle(event2)
	if err != nil {
		t.Fatalf("handleSessionLifecycle failed: %v", err)
	}

	expectedBucket := calculateBucket(authTime1, d.bucketSize)
	t.Logf("Sessions added to bucket %d (second :%02d), should fire in ~%d seconds",
		expectedBucket, targetSecond, (targetSecond-now.Second()+60)%60)

	d.BuildAccountingBuckets()

	time.Sleep(120 * time.Second)

	cancel()
}

func TestHandleSessionLifecycle(t *testing.T) {
	d := &Daemon{
		buckets:    make(map[int][]string),
		bucketSize: 5 * time.Second,
		acctCache:  make(map[string]*AccountingSession),
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
			event := models.Event{
				Type:       models.EventTypeSessionLifecycle,
				Timestamp:  tt.session.authTime,
				SessionID:  tt.session.sessionID,
				AccessType: models.AccessTypeIPoE,
				Protocol:   models.ProtocolDHCPv4,
			}
			event.SetPayload(tt.session.session)

			err := d.handleSessionLifecycle(event)
			if err != nil {
				t.Fatalf("handleSessionLifecycle() error = %v", err)
			}

			sessions := d.buckets[tt.wantBucket]
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
	t.Logf("Buckets: %+v", d.buckets)
}
