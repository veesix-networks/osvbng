package aaa

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/veesix-networks/osvbng/pkg/auth"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/provider"
	"github.com/veesix-networks/osvbng/pkg/southbound"
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

type stubShowSource struct {
	mu     sync.Mutex
	calls  int32
	result []southbound.InterfaceStats
	err    error
}

func (s *stubShowSource) Snapshot(_ context.Context, path string) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	atomic.AddInt32(&s.calls, 1)
	if path != paths.SystemDataplaneInterfaces.String() {
		return nil, errors.New("unexpected path")
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.result, nil
}

type recordingAuthProvider struct {
	mu          sync.Mutex
	lastSession auth.Session
	lastKind    string
	updateErr   error
}

func (*recordingAuthProvider) Info() provider.Info { return provider.Info{} }
func (*recordingAuthProvider) Authenticate(context.Context, *auth.AuthRequest) (*auth.AuthResponse, error) {
	return &auth.AuthResponse{}, nil
}
func (p *recordingAuthProvider) StartAccounting(_ context.Context, s *auth.Session) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastSession = *s
	p.lastKind = "start"
	return nil
}
func (p *recordingAuthProvider) UpdateAccounting(_ context.Context, s *auth.Session) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastSession = *s
	p.lastKind = "update"
	return p.updateErr
}
func (p *recordingAuthProvider) StopAccounting(_ context.Context, s *auth.Session) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastSession = *s
	p.lastKind = "stop"
	return nil
}

func (p *recordingAuthProvider) waitFor(t *testing.T, kind string) {
	t.Helper()
	for i := 0; i < 200; i++ {
		p.mu.Lock()
		got := p.lastKind
		p.mu.Unlock()
		if got == kind {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("auth provider never saw %q", kind)
}

func TestApplyVPPCountersFreshSession(t *testing.T) {
	s := &AccountingSession{}
	stats := &southbound.InterfaceStats{RxBytes: 1_000_000, TxBytes: 500_000, Rx: 800, Tx: 400}
	rxB, txB, rxP, txP := s.applyVPPCounters(stats)

	if rxB != 1_000_000 || txB != 500_000 || rxP != 800 || txP != 400 {
		t.Fatalf("fresh session cumulative wrong: got (%d, %d, %d, %d)", rxB, txB, rxP, txP)
	}
}

func TestApplyVPPCountersSteadyAdvance(t *testing.T) {
	s := &AccountingSession{
		lastReportedInOctets:  1_000_000,
		lastReportedOutOctets: 500_000,
	}
	stats := &southbound.InterfaceStats{RxBytes: 2_500_000, TxBytes: 1_300_000, Rx: 2_000, Tx: 1_000}
	rxB, txB, _, _ := s.applyVPPCounters(stats)

	if rxB != 2_500_000 || txB != 1_300_000 {
		t.Fatalf("steady advance wrong: got (%d, %d)", rxB, txB)
	}
}

func TestApplyVPPCountersRebaselineOnRegress(t *testing.T) {
	s := &AccountingSession{
		currentBaselineInBytes:    10_000_000,
		currentBaselineOutBytes:   5_000_000,
		currentBaselineInPackets:  10_000,
		currentBaselineOutPackets: 5_000,

		lastReportedInOctets:   11_000_000,
		lastReportedOutOctets:  5_500_000,
		lastReportedInPackets:  10_500,
		lastReportedOutPackets: 5_200,
	}
	stats := &southbound.InterfaceStats{RxBytes: 200, TxBytes: 100, Rx: 5, Tx: 3}
	rxB, txB, rxP, txP := s.applyVPPCounters(stats)

	if rxB != 11_000_200 {
		t.Fatalf("rebaseline RxBytes wrong: got %d, want 11_000_200", rxB)
	}
	if txB != 5_500_100 {
		t.Fatalf("rebaseline TxBytes wrong: got %d, want 5_500_100", txB)
	}
	if rxP != 10_505 {
		t.Fatalf("rebaseline RxPackets wrong: got %d, want 10_505", rxP)
	}
	if txP != 5_203 {
		t.Fatalf("rebaseline TxPackets wrong: got %d, want 5_203", txP)
	}
	if s.priorDeltaInBytes != 11_000_000 {
		t.Fatalf("priorDelta after rebaseline wrong: got %d, want 11_000_000", s.priorDeltaInBytes)
	}
	if s.currentBaselineInBytes != 0 {
		t.Fatalf("baseline after rebaseline wrong: got %d, want 0", s.currentBaselineInBytes)
	}
}

func TestApplyVPPCountersFreshAfterRebaseline(t *testing.T) {
	s := &AccountingSession{
		priorDeltaInBytes:    11_000_000,
		priorDeltaOutBytes:   5_500_000,
		priorDeltaInPackets:  10_500,
		priorDeltaOutPackets: 5_200,
		lastReportedInOctets: 11_000_200,
	}
	stats := &southbound.InterfaceStats{RxBytes: 500_000, TxBytes: 250_000, Rx: 400, Tx: 200}
	rxB, txB, rxP, txP := s.applyVPPCounters(stats)

	if rxB != 11_500_000 {
		t.Fatalf("post-rebaseline RxBytes wrong: got %d, want 11_500_000", rxB)
	}
	if txB != 5_750_000 {
		t.Fatalf("post-rebaseline TxBytes wrong: got %d, want 5_750_000", txB)
	}
	if rxP != 10_900 {
		t.Fatalf("post-rebaseline RxPackets wrong: got %d, want 10_900", rxP)
	}
	if txP != 5_400 {
		t.Fatalf("post-rebaseline TxPackets wrong: got %d, want 5_400", txP)
	}
}

func TestApplyVPPCountersGigawordsBoundary(t *testing.T) {
	const gigawordBoundary = uint64(1) << 32
	s := &AccountingSession{}
	stats := &southbound.InterfaceStats{RxBytes: gigawordBoundary + 12_345}
	rxB, _, _, _ := s.applyVPPCounters(stats)

	if rxB != gigawordBoundary+12_345 {
		t.Fatalf("gigawords boundary RxBytes wrong: got %d, want %d", rxB, gigawordBoundary+12_345)
	}
	if rxB>>32 != 1 {
		t.Fatalf("expected cumulative to cross 2^32, got %d (high32=%d)", rxB, rxB>>32)
	}
}

func TestAdvanceLastReported(t *testing.T) {
	s := &AccountingSession{}
	s.advanceLastReported(1_234_567, 8_910_111, 2_222, 3_333)

	if s.lastReportedInOctets != 1_234_567 || s.lastReportedOutOctets != 8_910_111 {
		t.Fatalf("advanceLastReported did not record octets: %+v", s)
	}
	if s.lastReportedInPackets != 2_222 || s.lastReportedOutPackets != 3_333 {
		t.Fatalf("advanceLastReported did not record packets: %+v", s)
	}
}

func newCounterTestComponent(t *testing.T, ap auth.AuthProvider, ss component.ShowSource) *Component {
	t.Helper()
	base := component.NewBase("aaa-test")
	base.StartContext(context.Background())
	t.Cleanup(base.StopContext)
	return &Component{
		Base:         base,
		logger:       logger.NewTest(),
		authProvider: ap,
		showSource:   ss,
		buckets:      make(map[int][]string),
		acctCache:    make(map[string]*AccountingSession),
	}
}

func TestSendAccountingUpdatePopulatesCumulative(t *testing.T) {
	ap := &recordingAuthProvider{}
	ss := &stubShowSource{result: []southbound.InterfaceStats{
		{Index: 42, Name: "ipoe_session0", RxBytes: 1_500_000, TxBytes: 700_000, Rx: 1_200, Tx: 600},
	}}
	c := newCounterTestComponent(t, ap, ss)

	acctSess := &AccountingSession{
		sessionID:     "abc",
		acctSessionID: "acct-abc",
		username:      "alice",
		mac:           "02:00:00:00:00:01",
		authDate:      time.Now().Add(-30 * time.Second),
		swIfIndex:     42,
		attributes:    map[string]string{},
	}
	c.acctCache[acctSess.sessionID] = acctSess
	c.placeSessionInBucket(acctSess.sessionID)

	bucketId := bucketForSession(acctSess.sessionID)
	c.ProcessAccountingBucket(bucketId)

	ap.waitFor(t, "update")

	ap.mu.Lock()
	defer ap.mu.Unlock()
	if ap.lastKind != "update" {
		t.Fatalf("UpdateAccounting was not called: lastKind=%q", ap.lastKind)
	}
	if ap.lastSession.RxBytes != 1_500_000 || ap.lastSession.TxBytes != 700_000 {
		t.Fatalf("auth.Session bytes wrong: got Rx=%d Tx=%d", ap.lastSession.RxBytes, ap.lastSession.TxBytes)
	}
	if ap.lastSession.RxPackets != 1_200 || ap.lastSession.TxPackets != 600 {
		t.Fatalf("auth.Session packets wrong: got Rx=%d Tx=%d", ap.lastSession.RxPackets, ap.lastSession.TxPackets)
	}
}

func TestSendAccountingUpdateSnapshotErrorEmitsLastReported(t *testing.T) {
	ap := &recordingAuthProvider{}
	ss := &stubShowSource{err: errors.New("stats segment unavailable")}
	c := newCounterTestComponent(t, ap, ss)

	acctSess := &AccountingSession{
		sessionID:              "abc",
		swIfIndex:              42,
		authDate:               time.Now(),
		lastReportedInOctets:   7_000_000,
		lastReportedOutOctets:  3_500_000,
		lastReportedInPackets:  5_000,
		lastReportedOutPackets: 2_500,
	}
	c.acctCache[acctSess.sessionID] = acctSess
	c.placeSessionInBucket(acctSess.sessionID)

	c.ProcessAccountingBucket(bucketForSession(acctSess.sessionID))
	ap.waitFor(t, "update")

	ap.mu.Lock()
	defer ap.mu.Unlock()
	if ap.lastSession.RxBytes != 7_000_000 || ap.lastSession.TxBytes != 3_500_000 {
		t.Fatalf("snapshot-error fallback wrong: got Rx=%d Tx=%d", ap.lastSession.RxBytes, ap.lastSession.TxBytes)
	}
	if ap.lastSession.RxPackets != 5_000 || ap.lastSession.TxPackets != 2_500 {
		t.Fatalf("snapshot-error packets fallback wrong: got Rx=%d Tx=%d", ap.lastSession.RxPackets, ap.lastSession.TxPackets)
	}
}

func TestSendAccountingUpdateMissingIfIndexEmitsLastReported(t *testing.T) {
	ap := &recordingAuthProvider{}
	ss := &stubShowSource{result: []southbound.InterfaceStats{
		{Index: 7, Name: "unrelated"},
	}}
	c := newCounterTestComponent(t, ap, ss)

	acctSess := &AccountingSession{
		sessionID:             "abc",
		swIfIndex:             42,
		authDate:              time.Now(),
		lastReportedInOctets:  7_000_000,
		lastReportedOutOctets: 3_500_000,
	}
	c.acctCache[acctSess.sessionID] = acctSess
	c.placeSessionInBucket(acctSess.sessionID)

	c.ProcessAccountingBucket(bucketForSession(acctSess.sessionID))
	ap.waitFor(t, "update")

	ap.mu.Lock()
	defer ap.mu.Unlock()
	if ap.lastSession.RxBytes != 7_000_000 || ap.lastSession.TxBytes != 3_500_000 {
		t.Fatalf("missing-ifIndex fallback wrong: got Rx=%d Tx=%d", ap.lastSession.RxBytes, ap.lastSession.TxBytes)
	}
}

func TestSendAccountingUpdateAdvancesLastReportedOnSuccess(t *testing.T) {
	ap := &recordingAuthProvider{}
	ss := &stubShowSource{result: []southbound.InterfaceStats{
		{Index: 42, RxBytes: 2_000_000, TxBytes: 1_000_000, Rx: 1_500, Tx: 800},
	}}
	c := newCounterTestComponent(t, ap, ss)

	acctSess := &AccountingSession{sessionID: "abc", swIfIndex: 42, authDate: time.Now()}
	c.acctCache[acctSess.sessionID] = acctSess
	c.placeSessionInBucket(acctSess.sessionID)

	c.ProcessAccountingBucket(bucketForSession(acctSess.sessionID))
	ap.waitFor(t, "update")

	acctSess.mu.Lock()
	defer acctSess.mu.Unlock()
	if acctSess.lastReportedInOctets != 2_000_000 || acctSess.lastReportedOutOctets != 1_000_000 {
		t.Fatalf("LastReported did not advance: %+v", acctSess)
	}
}

func TestSendAccountingUpdateDoesNotAdvanceOnError(t *testing.T) {
	ap := &recordingAuthProvider{updateErr: errors.New("transport timeout")}
	ss := &stubShowSource{result: []southbound.InterfaceStats{
		{Index: 42, RxBytes: 2_000_000, TxBytes: 1_000_000},
	}}
	c := newCounterTestComponent(t, ap, ss)

	acctSess := &AccountingSession{sessionID: "abc", swIfIndex: 42, authDate: time.Now()}
	c.acctCache[acctSess.sessionID] = acctSess
	c.placeSessionInBucket(acctSess.sessionID)

	c.ProcessAccountingBucket(bucketForSession(acctSess.sessionID))
	ap.waitFor(t, "update")

	acctSess.mu.Lock()
	defer acctSess.mu.Unlock()
	if acctSess.lastReportedInOctets != 0 {
		t.Fatalf("LastReported advanced despite error: %d", acctSess.lastReportedInOctets)
	}
}

func TestHandleSessionReleaseEmitsFinalCumulative(t *testing.T) {
	ap := &recordingAuthProvider{}
	ss := &stubShowSource{result: []southbound.InterfaceStats{
		{Index: 42, RxBytes: 9_000_000, TxBytes: 4_000_000, Rx: 6_000, Tx: 3_000},
	}}
	c := newCounterTestComponent(t, ap, ss)

	acctSess := &AccountingSession{
		sessionID: "abc", swIfIndex: 42,
		authDate: time.Now().Add(-1 * time.Minute),
	}
	c.acctCache[acctSess.sessionID] = acctSess
	c.placeSessionInBucket(acctSess.sessionID)

	if err := c.handleSessionRelease("abc", "alice", "02:00:00:00:00:01", "acct-abc", nil); err != nil {
		t.Fatalf("handleSessionRelease: %v", err)
	}
	ap.waitFor(t, "stop")

	ap.mu.Lock()
	defer ap.mu.Unlock()
	if ap.lastKind != "stop" {
		t.Fatalf("StopAccounting was not called: lastKind=%q", ap.lastKind)
	}
	if ap.lastSession.RxBytes != 9_000_000 || ap.lastSession.TxBytes != 4_000_000 {
		t.Fatalf("Acct-Stop bytes wrong: got Rx=%d Tx=%d", ap.lastSession.RxBytes, ap.lastSession.TxBytes)
	}
	if ap.lastSession.RxPackets != 6_000 || ap.lastSession.TxPackets != 3_000 {
		t.Fatalf("Acct-Stop packets wrong: got Rx=%d Tx=%d", ap.lastSession.RxPackets, ap.lastSession.TxPackets)
	}
}

func TestHandleSessionLifecycleAcctStartCarriesZeros(t *testing.T) {
	ap := &recordingAuthProvider{}
	c := newCounterTestComponent(t, ap, nil)

	ts := createTestSession("abc", time.Now(), "02:00:00:00:00:01", "10.1.0.2")
	c.handleSessionLifecycle(events.Event{
		Timestamp: ts.authTime,
		Data: &events.SessionLifecycleEvent{
			AccessType: models.AccessTypeIPoE,
			Protocol:   models.ProtocolDHCPv4,
			SessionID:  ts.sessionID,
			State:      ts.session.State,
			Session:    ts.session,
		},
	})
	ap.waitFor(t, "start")

	ap.mu.Lock()
	defer ap.mu.Unlock()
	if ap.lastKind != "start" {
		t.Fatalf("StartAccounting was not called: lastKind=%q", ap.lastKind)
	}
	if ap.lastSession.RxBytes != 0 || ap.lastSession.TxBytes != 0 ||
		ap.lastSession.RxPackets != 0 || ap.lastSession.TxPackets != 0 {
		t.Fatalf("Acct-Start must carry zero counters, got %+v", ap.lastSession)
	}
}
