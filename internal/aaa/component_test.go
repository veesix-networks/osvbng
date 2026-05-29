package aaa

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/veesix-networks/osvbng/pkg/auth"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/logger"
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

func TestBucketForSessionDeterministic(t *testing.T) {
	const sid = "8a179135-4a4d-4ac0-89ba-827618ebbf57"
	first := bucketForSession(sid)
	for i := 0; i < 100; i++ {
		if got := bucketForSession(sid); got != first {
			t.Fatalf("bucketForSession(%q) returned %d after %d (call %d)", sid, got, first, i)
		}
	}
	if first < 0 || first >= numBuckets() {
		t.Fatalf("bucket %d out of range [0,%d)", first, numBuckets())
	}
}

func TestBucketForSessionDistribution(t *testing.T) {
	const (
		samples = 1200
		buckets = 12
	)
	hist := make(map[int]int, buckets)
	for i := 0; i < samples; i++ {
		sid := fmt.Sprintf("session-%08d-%d", i, i*7919)
		hist[bucketForSession(sid)]++
	}
	if len(hist) != buckets {
		t.Fatalf("expected all %d buckets to receive at least one session, got %d distinct buckets", buckets, len(hist))
	}
	min, max := samples, 0
	for _, count := range hist {
		if count < min {
			min = count
		}
		if count > max {
			max = count
		}
	}
	if max > 2*min {
		t.Fatalf("FNV-1a distribution skewed beyond 2x: min=%d max=%d hist=%v", min, max, hist)
	}
}

func TestPlaceSessionInBucketSweepsStale(t *testing.T) {
	c := &Component{
		buckets: make(map[int][]string),
	}
	const sid = "8a179135-4a4d-4ac0-89ba-827618ebbf57"
	target := bucketForSession(sid)
	stale := (target + 1) % numBuckets()

	c.buckets[stale] = []string{sid}

	got, already := c.placeSessionInBucket(sid)
	if got != target {
		t.Fatalf("placeSessionInBucket returned bucket %d, want %d", got, target)
	}
	if already {
		t.Fatalf("session was in the wrong bucket; alreadyPresent must be false so the caller treats it as a fresh placement")
	}
	if n := len(c.buckets[stale]); n != 0 {
		t.Fatalf("stale bucket %d still has %d entries after sweep: %v", stale, n, c.buckets[stale])
	}
	if len(c.buckets[target]) != 1 || c.buckets[target][0] != sid {
		t.Fatalf("target bucket %d does not contain session: %v", target, c.buckets[target])
	}

	got, already = c.placeSessionInBucket(sid)
	if got != target {
		t.Fatalf("second call: bucket %d, want %d", got, target)
	}
	if !already {
		t.Fatalf("second call: alreadyPresent must be true")
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
		logger:       logger.NewTest(),
		authProvider: noopAuthProvider{},
		buckets:      make(map[int][]string),
		acctCache:    make(map[string]*AccountingSession),
	}

	sessions := []testSession{
		createTestSession("8a179135-4a4d-4ac0-89ba-827618ebbf57", time.Date(2025, 12, 13, 10, 23, 0, 500000000, time.UTC), "02:00:00:00:00:01", "10.1.0.163"),
		createTestSession("7b289246-5b5e-4bd1-9acb-938729fccg68", time.Date(2025, 12, 13, 10, 23, 3, 440000000, time.UTC), "02:00:00:00:00:02", "10.1.0.164"),
		createTestSession("9c390357-6c6f-5ce2-abdc-a49830gddh79", time.Date(2025, 12, 13, 10, 23, 7, 800000000, time.UTC), "02:00:00:00:00:03", "10.1.0.165"),
		createTestSession("ad4a1468-7d7g-6df3-bced-b5a941heei8a", time.Date(2025, 12, 13, 10, 23, 59, 900000000, time.UTC), "02:00:00:00:00:04", "10.1.0.166"),
	}

	for _, s := range sessions {
		t.Run(s.sessionID, func(t *testing.T) {
			event := events.Event{
				Timestamp: s.authTime,
				Data: &events.SessionLifecycleEvent{
					AccessType: models.AccessTypeIPoE,
					Protocol:   models.ProtocolDHCPv4,
					SessionID:  s.sessionID,
					State:      s.session.State,
					Session:    s.session,
				},
			}

			c.handleSessionLifecycle(event)

			wantBucket := bucketForSession(s.sessionID)
			ids := c.buckets[wantBucket]
			if len(ids) == 0 {
				t.Fatalf("expected session in bucket %d, but bucket is empty", wantBucket)
			}
			found := false
			for _, sid := range ids {
				if sid == s.sessionID {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("session %s not found in its deterministic bucket %d", s.sessionID, wantBucket)
			}

			for bid, bucketSids := range c.buckets {
				if bid == wantBucket {
					continue
				}
				for _, sid := range bucketSids {
					if sid == s.sessionID {
						t.Errorf("session %s also in non-deterministic bucket %d (should only be in %d)", s.sessionID, bid, wantBucket)
					}
				}
			}
		})
	}
	t.Logf("Buckets: %+v", c.buckets)
}

func TestHandleSessionLifecycleNoDuplicateOnRepublish(t *testing.T) {
	base := component.NewBase("aaa-test")
	base.StartContext(context.Background())
	defer base.StopContext()

	c := &Component{
		Base:         base,
		logger:       logger.NewTest(),
		authProvider: noopAuthProvider{},
		buckets:      make(map[int][]string),
		acctCache:    make(map[string]*AccountingSession),
	}

	s := createTestSession("8a179135-4a4d-4ac0-89ba-827618ebbf57", time.Date(2025, 12, 13, 10, 23, 0, 500000000, time.UTC), "02:00:00:00:00:01", "10.1.0.163")

	for i := 0; i < 5; i++ {
		event := events.Event{
			Timestamp: s.authTime.Add(time.Duration(i*7) * time.Second),
			Data: &events.SessionLifecycleEvent{
				AccessType: models.AccessTypeIPoE,
				Protocol:   models.ProtocolDHCPv4,
				SessionID:  s.sessionID,
				State:      s.session.State,
				Session:    s.session,
			},
		}
		c.handleSessionLifecycle(event)
	}

	count := 0
	for _, ids := range c.buckets {
		for _, sid := range ids {
			if sid == s.sessionID {
				count++
			}
		}
	}
	if count != 1 {
		t.Fatalf("expected session in exactly 1 bucket across 5 republishes, got %d entries; buckets=%+v", count, c.buckets)
	}
}
