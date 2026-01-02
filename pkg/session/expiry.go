package session

import (
	"container/heap"
	"context"
	"sync"
	"time"
)

type ExpiryCallback func(sessionID string, expiryTime time.Time)

type expiryEntry struct {
	sessionID  string
	expiryTime time.Time
	index      int
}

type expiryHeap []*expiryEntry

func (h expiryHeap) Len() int           { return len(h) }
func (h expiryHeap) Less(i, j int) bool { return h[i].expiryTime.Before(h[j].expiryTime) }
func (h expiryHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *expiryHeap) Push(x interface{}) {
	n := len(*h)
	entry := x.(*expiryEntry)
	entry.index = n
	*h = append(*h, entry)
}

func (h *expiryHeap) Pop() interface{} {
	old := *h
	n := len(old)
	entry := old[n-1]
	old[n-1] = nil
	entry.index = -1
	*h = old[0 : n-1]
	return entry
}

type ExpiryManager struct {
	heap     expiryHeap
	sessions map[string]*expiryEntry
	mu       sync.Mutex
	wakeup   chan struct{}
	callback ExpiryCallback
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

func NewExpiryManager(callback ExpiryCallback) *ExpiryManager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &ExpiryManager{
		heap:     make(expiryHeap, 0),
		sessions: make(map[string]*expiryEntry),
		wakeup:   make(chan struct{}, 1),
		callback: callback,
		ctx:      ctx,
		cancel:   cancel,
	}
	heap.Init(&m.heap)
	return m
}

func (m *ExpiryManager) Start() {
	m.wg.Add(1)
	go m.run()
}

func (m *ExpiryManager) Stop() {
	m.cancel()
	m.wg.Wait()
}

func (m *ExpiryManager) Set(sessionID string, expiryTime time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if entry, exists := m.sessions[sessionID]; exists {
		entry.expiryTime = expiryTime
		heap.Fix(&m.heap, entry.index)
	} else {
		entry := &expiryEntry{
			sessionID:  sessionID,
			expiryTime: expiryTime,
		}
		heap.Push(&m.heap, entry)
		m.sessions[sessionID] = entry
	}

	select {
	case m.wakeup <- struct{}{}:
	default:
	}
}

func (m *ExpiryManager) Remove(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if entry, exists := m.sessions[sessionID]; exists {
		heap.Remove(&m.heap, entry.index)
		delete(m.sessions, sessionID)
	}
}

func (m *ExpiryManager) run() {
	defer m.wg.Done()

	var timer *time.Timer
	var timerCh <-chan time.Time

	for {
		m.mu.Lock()
		var nextWake time.Duration
		if m.heap.Len() > 0 {
			nextExpiry := m.heap[0].expiryTime
			now := time.Now()
			if nextExpiry.Before(now) || nextExpiry.Equal(now) {
				entry := heap.Pop(&m.heap).(*expiryEntry)
				delete(m.sessions, entry.sessionID)
				m.mu.Unlock()

				if m.callback != nil {
					m.callback(entry.sessionID, entry.expiryTime)
				}
				continue
			}
			nextWake = nextExpiry.Sub(now)
		} else {
			nextWake = time.Hour * 24
		}
		m.mu.Unlock()

		if timer == nil {
			timer = time.NewTimer(nextWake)
			timerCh = timer.C
		} else {
			timer.Reset(nextWake)
		}

		select {
		case <-m.ctx.Done():
			timer.Stop()
			return
		case <-timerCh:
		case <-m.wakeup:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
	}
}
