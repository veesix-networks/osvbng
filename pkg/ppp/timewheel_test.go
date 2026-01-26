package ppp

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestNewTimeWheel(t *testing.T) {
	tw := NewTimeWheel(60, time.Second, nil)
	if tw.numBuckets != 60 {
		t.Errorf("expected 60 buckets, got %d", tw.numBuckets)
	}
	if tw.Count() != 0 {
		t.Errorf("expected 0 sessions, got %d", tw.Count())
	}
}

func TestTimeWheelAdd(t *testing.T) {
	tw := NewTimeWheel(10, time.Second, nil)

	state := &EchoState{
		SessionID: 100,
		Magic:     0x12345678,
	}
	tw.Add(100, state)

	if tw.Count() != 1 {
		t.Errorf("expected 1 session, got %d", tw.Count())
	}

	got := tw.Get(100)
	if got == nil {
		t.Fatal("expected to find session 100")
	}
	if got.Magic != 0x12345678 {
		t.Errorf("expected magic 0x12345678, got 0x%x", got.Magic)
	}
}

func TestTimeWheelRemove(t *testing.T) {
	tw := NewTimeWheel(10, time.Second, nil)

	tw.Add(100, &EchoState{SessionID: 100})
	tw.Add(200, &EchoState{SessionID: 200})

	if tw.Count() != 2 {
		t.Errorf("expected 2 sessions, got %d", tw.Count())
	}

	tw.Remove(100)

	if tw.Count() != 1 {
		t.Errorf("expected 1 session after remove, got %d", tw.Count())
	}

	if tw.Get(100) != nil {
		t.Error("session 100 should be removed")
	}
	if tw.Get(200) == nil {
		t.Error("session 200 should still exist")
	}
}

func TestTimeWheelUpdateLastSeen(t *testing.T) {
	tw := NewTimeWheel(10, time.Second, nil)

	state := &EchoState{
		SessionID: 100,
		MissCount: 3,
		LastSeen:  time.Now().Add(-time.Hour),
	}
	tw.Add(100, state)

	tw.UpdateLastSeen(100)

	got := tw.Get(100)
	if got.MissCount != 0 {
		t.Errorf("expected MissCount reset to 0, got %d", got.MissCount)
	}
	if time.Since(got.LastSeen) > time.Second {
		t.Error("LastSeen should be recent")
	}
}

func TestTimeWheelTick(t *testing.T) {
	tw := NewTimeWheel(3, time.Second, nil)

	tw.Add(0, &EchoState{SessionID: 0})
	tw.Add(1, &EchoState{SessionID: 1})
	tw.Add(2, &EchoState{SessionID: 2})
	tw.Add(3, &EchoState{SessionID: 3})

	if tw.Count() != 4 {
		t.Errorf("expected 4 sessions, got %d", tw.Count())
	}

	totalSeen := 0
	for i := 0; i < 3; i++ {
		sessions := tw.Tick()
		totalSeen += len(sessions)
	}

	if totalSeen != 4 {
		t.Errorf("expected to see all 4 sessions after full rotation, got %d", totalSeen)
	}

	firstBucketSessions := tw.Tick()
	secondRotationSeen := len(firstBucketSessions)
	for i := 0; i < 2; i++ {
		secondRotationSeen += len(tw.Tick())
	}

	if secondRotationSeen != 4 {
		t.Errorf("expected to see all 4 sessions on second rotation, got %d", secondRotationSeen)
	}
}

func TestTimeWheelAddToBucket(t *testing.T) {
	tw := NewTimeWheel(10, time.Second, nil)

	state := &EchoState{SessionID: 999}
	tw.AddToBucket(5, state)

	tw.current = 5
	sessions := tw.Tick()
	if len(sessions) != 1 {
		t.Errorf("expected 1 session in bucket 5, got %d", len(sessions))
	}
	if sessions[0].SessionID != 999 {
		t.Errorf("expected session 999, got %d", sessions[0].SessionID)
	}
}

func TestTimeWheelCallback(t *testing.T) {
	var callCount atomic.Int32
	var lastBatchSize atomic.Int32

	tw := NewTimeWheel(2, 10*time.Millisecond, func(sessions []*EchoState) {
		callCount.Add(1)
		lastBatchSize.Store(int32(len(sessions)))
	})

	tw.Add(0, &EchoState{SessionID: 0})
	tw.Add(2, &EchoState{SessionID: 2})
	tw.Add(1, &EchoState{SessionID: 1})

	go tw.Start()
	time.Sleep(35 * time.Millisecond)
	tw.Stop()

	if callCount.Load() < 2 {
		t.Errorf("expected at least 2 callbacks, got %d", callCount.Load())
	}
}

func TestTimeWheelStopIdempotent(t *testing.T) {
	tw := NewTimeWheel(10, time.Second, nil)

	go tw.Start()
	time.Sleep(10 * time.Millisecond)

	tw.Stop()
	tw.Stop()
}

func TestTimeWheelEmptyTick(t *testing.T) {
	tw := NewTimeWheel(10, time.Second, nil)

	sessions := tw.Tick()
	if sessions != nil {
		t.Errorf("expected nil for empty bucket, got %v", sessions)
	}
}

func TestTimeWheelDistribution(t *testing.T) {
	tw := NewTimeWheel(60, time.Second, nil)

	for i := 0; i < 1000; i++ {
		tw.Add(uint16(i), &EchoState{SessionID: uint16(i)})
	}

	if tw.Count() != 1000 {
		t.Errorf("expected 1000 sessions, got %d", tw.Count())
	}
}
