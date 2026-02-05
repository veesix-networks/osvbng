package vpp

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/veesix-networks/osvbng/pkg/gomemif/memif"
	"github.com/veesix-networks/osvbng/pkg/logger"
)

type EgressBatcher struct {
	highPriority chan []memif.MemifPacketBuffer
	lowPriority  chan []memif.MemifPacketBuffer

	highDrops    atomic.Uint64
	lowDrops     atomic.Uint64
	packetsSent  atomic.Uint64
	ringFullEvts atomic.Uint64

	paused atomic.Bool
	logger *slog.Logger
}

func NewEgressBatcher(highCap, lowCap int) *EgressBatcher {
	return &EgressBatcher{
		highPriority: make(chan []memif.MemifPacketBuffer, highCap),
		lowPriority:  make(chan []memif.MemifPacketBuffer, lowCap),
		logger:       logger.Get(logger.Egress),
	}
}

func (e *EgressBatcher) QueueHighPriority(batch []memif.MemifPacketBuffer) bool {
	if e.paused.Load() {
		e.highDrops.Add(uint64(len(batch)))
		return false
	}

	select {
	case e.highPriority <- batch:
		return true
	default:
		e.highDrops.Add(uint64(len(batch)))
		return false
	}
}

func (e *EgressBatcher) QueueLowPriority(batch []memif.MemifPacketBuffer) bool {
	if e.paused.Load() {
		e.lowDrops.Add(uint64(len(batch)))
		return false
	}

	select {
	case e.lowPriority <- batch:
		return true
	default:
		e.lowDrops.Add(uint64(len(batch)))
		return false
	}
}

func (e *EgressBatcher) WriteLoop(ctx context.Context, txQueue *memif.Queue) {
	backoff := time.Millisecond

	for {
		select {
		case batch := <-e.highPriority:
			e.writeBatch(txQueue, batch, &backoff)
			continue
		default:
		}

		select {
		case batch := <-e.highPriority:
			e.writeBatch(txQueue, batch, &backoff)
		case batch := <-e.lowPriority:
			e.writeBatch(txQueue, batch, &backoff)
		case <-ctx.Done():
			return
		}
	}
}

func (e *EgressBatcher) writeBatch(txQueue *memif.Queue, batch []memif.MemifPacketBuffer, backoff *time.Duration) {
	sent := txQueue.Tx_burst(batch)
	e.packetsSent.Add(uint64(sent))

	if sent < len(batch) {
		e.ringFullEvts.Add(1)
		time.Sleep(*backoff)
		*backoff = min(*backoff*2, 100*time.Millisecond)
	} else {
		*backoff = time.Millisecond
	}
}

func (e *EgressBatcher) Pause() {
	e.paused.Store(true)

	for len(e.highPriority) > 0 {
		<-e.highPriority
	}
	for len(e.lowPriority) > 0 {
		<-e.lowPriority
	}
}

func (e *EgressBatcher) Resume() {
	e.paused.Store(false)
}

func (e *EgressBatcher) IsPaused() bool {
	return e.paused.Load()
}

func (e *EgressBatcher) Metrics() map[string]uint64 {
	return map[string]uint64{
		"egress_packets_sent":      e.packetsSent.Load(),
		"egress_high_drops":        e.highDrops.Load(),
		"egress_low_drops":         e.lowDrops.Load(),
		"egress_ring_full_events":  e.ringFullEvts.Load(),
		"egress_high_queue_depth":  uint64(len(e.highPriority)),
		"egress_low_queue_depth":   uint64(len(e.lowPriority)),
	}
}

func (e *EgressBatcher) HighQueueDepth() int {
	return len(e.highPriority)
}

func (e *EgressBatcher) LowQueueDepth() int {
	return len(e.lowPriority)
}
