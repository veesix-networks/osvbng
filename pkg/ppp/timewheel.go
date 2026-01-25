package ppp

import (
	"sync"
	"time"
)

type EchoState struct {
	SessionID  uint16
	Magic      uint32
	LastEchoID uint8
	MissCount  int
	LastSeen   time.Time
	// Frame building info
	DstMAC    [6]byte
	SrcMAC    [6]byte
	OuterVLAN uint16 // 0 = no outer VLAN
	InnerVLAN uint16 // 0 = no inner VLAN
}

type TimeWheel struct {
	buckets    []map[uint16]*EchoState
	current    int
	interval   time.Duration
	numBuckets int
	mu         sync.RWMutex
	onExpire   func(sessions []*EchoState)
	stopCh     chan struct{}
	stopped    bool
}

func NewTimeWheel(numBuckets int, interval time.Duration, onExpire func(sessions []*EchoState)) *TimeWheel {
	buckets := make([]map[uint16]*EchoState, numBuckets)
	for i := range buckets {
		buckets[i] = make(map[uint16]*EchoState)
	}

	return &TimeWheel{
		buckets:    buckets,
		numBuckets: numBuckets,
		interval:   interval,
		onExpire:   onExpire,
		stopCh:     make(chan struct{}),
	}
}

func (tw *TimeWheel) Add(sessionID uint16, state *EchoState) {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	bucket := int(sessionID) % tw.numBuckets
	tw.buckets[bucket][sessionID] = state
}

func (tw *TimeWheel) AddToBucket(bucket int, state *EchoState) {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	if bucket >= tw.numBuckets {
		bucket = bucket % tw.numBuckets
	}
	tw.buckets[bucket][state.SessionID] = state
}

func (tw *TimeWheel) Remove(sessionID uint16) {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	bucket := int(sessionID) % tw.numBuckets
	delete(tw.buckets[bucket], sessionID)
}

func (tw *TimeWheel) Get(sessionID uint16) *EchoState {
	tw.mu.RLock()
	defer tw.mu.RUnlock()

	bucket := int(sessionID) % tw.numBuckets
	return tw.buckets[bucket][sessionID]
}

func (tw *TimeWheel) UpdateLastSeen(sessionID uint16) {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	bucket := int(sessionID) % tw.numBuckets
	if state, ok := tw.buckets[bucket][sessionID]; ok {
		state.LastSeen = time.Now()
		state.MissCount = 0
	}
}

func (tw *TimeWheel) Tick() []*EchoState {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	bucket := tw.buckets[tw.current]
	tw.current = (tw.current + 1) % tw.numBuckets

	if len(bucket) == 0 {
		return nil
	}

	sessions := make([]*EchoState, 0, len(bucket))
	for _, state := range bucket {
		sessions = append(sessions, state)
	}

	return sessions
}

func (tw *TimeWheel) Start() {
	ticker := time.NewTicker(tw.interval)
	defer ticker.Stop()

	for {
		select {
		case <-tw.stopCh:
			return
		case <-ticker.C:
			sessions := tw.Tick()
			if tw.onExpire != nil && len(sessions) > 0 {
				tw.onExpire(sessions)
			}
		}
	}
}

func (tw *TimeWheel) Stop() {
	tw.mu.Lock()
	if tw.stopped {
		tw.mu.Unlock()
		return
	}
	tw.stopped = true
	tw.mu.Unlock()

	close(tw.stopCh)
}

func (tw *TimeWheel) Count() int {
	tw.mu.RLock()
	defer tw.mu.RUnlock()

	total := 0
	for _, bucket := range tw.buckets {
		total += len(bucket)
	}
	return total
}
