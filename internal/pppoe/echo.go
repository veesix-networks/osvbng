package pppoe

import (
	"encoding/binary"
	"hash/fnv"
	"log/slog"
	"sync"
	"time"

	"github.com/veesix-networks/osvbng/pkg/gomemif/memif"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/ppp"
)

type EchoGenerator struct {
	timeWheel   *ppp.TimeWheel
	batcher     BatchSender
	bufferPool  sync.Pool
	interval    time.Duration
	maxMisses   int
	maxPerTick  int
	logger      *slog.Logger
	onDeadPeer  func(sessionID uint16)
}

type BatchSender interface {
	QueueLowPriority(batch []memif.MemifPacketBuffer) bool
}

type EchoConfig struct {
	Interval    time.Duration
	MaxMisses   int
	MaxPerTick  int
	NumBuckets  int
}

func DefaultEchoConfig() EchoConfig {
	return EchoConfig{
		Interval:   30 * time.Second,
		MaxMisses:  3,
		MaxPerTick: 5000,
		NumBuckets: 60,
	}
}

func NewEchoGenerator(cfg EchoConfig, batcher BatchSender, onDeadPeer func(uint16)) *EchoGenerator {
	g := &EchoGenerator{
		batcher:    batcher,
		interval:   cfg.Interval,
		maxMisses:  cfg.MaxMisses,
		maxPerTick: cfg.MaxPerTick,
		logger:     logger.Component(logger.ComponentPPPoE),
		onDeadPeer: onDeadPeer,
		bufferPool: sync.Pool{
			New: func() interface{} {
				return make([]memif.MemifPacketBuffer, 0, 256)
			},
		},
	}

	tickInterval := cfg.Interval / time.Duration(cfg.NumBuckets)
	if tickInterval < 100*time.Millisecond {
		tickInterval = 100 * time.Millisecond
	}

	g.timeWheel = ppp.NewTimeWheel(cfg.NumBuckets, tickInterval, g.processTick)
	return g
}

func (g *EchoGenerator) Start() {
	go g.timeWheel.Start()
}

func (g *EchoGenerator) Stop() {
	g.timeWheel.Stop()
}

func (g *EchoGenerator) AddSession(sessionID uint16, magic uint32, dstMAC, srcMAC []byte, outerVLAN, innerVLAN uint16) {
	state := &ppp.EchoState{
		SessionID:  sessionID,
		Magic:      magic,
		LastEchoID: 0,
		MissCount:  0,
		LastSeen:   time.Now(),
		OuterVLAN:  outerVLAN,
		InnerVLAN:  innerVLAN,
	}
	copy(state.DstMAC[:], dstMAC)
	copy(state.SrcMAC[:], srcMAC)

	h := fnv.New32a()
	h.Write([]byte{byte(sessionID), byte(sessionID >> 8)})
	bucket := int(h.Sum32()) % g.timeWheel.Count()
	if bucket < 0 {
		bucket = 0
	}

	g.timeWheel.AddToBucket(bucket, state)
}

func (g *EchoGenerator) RemoveSession(sessionID uint16) {
	g.timeWheel.Remove(sessionID)
}

func (g *EchoGenerator) HandleEchoReply(sessionID uint16, echoID uint8) {
	state := g.timeWheel.Get(sessionID)
	if state == nil {
		return
	}

	if state.LastEchoID == echoID {
		g.timeWheel.UpdateLastSeen(sessionID)
	}
}

func (g *EchoGenerator) RecordActivity(sessionID uint16) {
	g.timeWheel.UpdateLastSeen(sessionID)
}

func (g *EchoGenerator) processTick(sessions []*ppp.EchoState) {
	if len(sessions) == 0 {
		return
	}

	if len(sessions) > g.maxPerTick {
		g.logger.Warn("Rate limiting echo generation",
			"due", len(sessions),
			"limit", g.maxPerTick,
			"deferred", len(sessions)-g.maxPerTick)
		sessions = sessions[:g.maxPerTick]
	}

	var deadPeers []uint16
	var toSend []*ppp.EchoState

	for _, state := range sessions {
		if state.MissCount >= g.maxMisses {
			deadPeers = append(deadPeers, state.SessionID)
		} else {
			state.MissCount++
			state.LastEchoID++
			toSend = append(toSend, state)
		}
	}

	for _, sessionID := range deadPeers {
		g.timeWheel.Remove(sessionID)
		if g.onDeadPeer != nil {
			g.onDeadPeer(sessionID)
		}
	}

	if len(toSend) == 0 {
		return
	}

	batch := g.bufferPool.Get().([]memif.MemifPacketBuffer)
	batch = batch[:0]

	for _, state := range toSend {
		frame := g.buildEchoRequest(state)
		if frame != nil {
			batch = append(batch, memif.MemifPacketBuffer{
				Buf:    frame,
				Buflen: len(frame),
			})
		}
	}

	if len(batch) > 0 {
		if !g.batcher.QueueLowPriority(batch) {
			g.logger.Warn("Egress backpressure, dropped echo batch", "count", len(batch))
		}
	}
}

func (g *EchoGenerator) buildEchoRequest(state *ppp.EchoState) []byte {
	lcpPayload := make([]byte, 8)
	lcpPayload[0] = ppp.EchoReq
	lcpPayload[1] = state.LastEchoID
	binary.BigEndian.PutUint16(lcpPayload[2:4], 8)
	binary.BigEndian.PutUint32(lcpPayload[4:8], state.Magic)

	pppPayload := make([]byte, 2+len(lcpPayload))
	binary.BigEndian.PutUint16(pppPayload[0:2], ppp.ProtoLCP)
	copy(pppPayload[2:], lcpPayload)

	pppoeHdr := make([]byte, 6)
	pppoeHdr[0] = 0x11
	pppoeHdr[1] = 0x00
	binary.BigEndian.PutUint16(pppoeHdr[2:4], state.SessionID)
	binary.BigEndian.PutUint16(pppoeHdr[4:6], uint16(len(pppPayload)))

	vlanLen := 0
	if state.OuterVLAN != 0 {
		vlanLen = 4
		if state.InnerVLAN != 0 {
			vlanLen = 8
		}
	}
	frame := make([]byte, 14+vlanLen+6+len(pppPayload))
	off := 0

	copy(frame[off:], state.DstMAC[:])
	off += 6
	copy(frame[off:], state.SrcMAC[:])
	off += 6

	if state.OuterVLAN != 0 {
		if state.InnerVLAN != 0 {
			binary.BigEndian.PutUint16(frame[off:], 0x88a8)
			binary.BigEndian.PutUint16(frame[off+2:], state.OuterVLAN)
			binary.BigEndian.PutUint16(frame[off+4:], 0x8100)
			binary.BigEndian.PutUint16(frame[off+6:], state.InnerVLAN)
			off += 8
		} else {
			binary.BigEndian.PutUint16(frame[off:], 0x8100)
			binary.BigEndian.PutUint16(frame[off+2:], state.OuterVLAN)
			off += 4
		}
	}

	binary.BigEndian.PutUint16(frame[off:], 0x8864)
	off += 2

	copy(frame[off:], pppoeHdr)
	off += 6
	copy(frame[off:], pppPayload)

	return frame
}

func (g *EchoGenerator) SessionCount() int {
	return g.timeWheel.Count()
}
