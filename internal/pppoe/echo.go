package pppoe

import (
	"log/slog"
	"time"

	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/ppp"
)

type EchoGenerator struct {
	timeWheel  *ppp.TimeWheel
	sendEcho   func(sessionID uint16, echoID uint8)
	onDeadPeer func(sessionID uint16)
	interval   time.Duration
	maxMisses  int
	maxPerTick int
	logger     *slog.Logger
}

type EchoConfig struct {
	Interval   time.Duration
	MaxMisses  int
	MaxPerTick int
	NumBuckets int
}

func DefaultEchoConfig() EchoConfig {
	return EchoConfig{
		Interval:   30 * time.Second,
		MaxMisses:  3,
		MaxPerTick: 5000,
		NumBuckets: 60,
	}
}

func NewEchoGenerator(cfg EchoConfig, sendEcho func(uint16, uint8), onDeadPeer func(uint16)) *EchoGenerator {
	g := &EchoGenerator{
		sendEcho:   sendEcho,
		onDeadPeer: onDeadPeer,
		interval:   cfg.Interval,
		maxMisses:  cfg.MaxMisses,
		maxPerTick: cfg.MaxPerTick,
		logger:     logger.Component(logger.ComponentPPPoE),
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

func (g *EchoGenerator) AddSession(sessionID uint16, magic uint32) {
	state := &ppp.EchoState{
		SessionID:  sessionID,
		Magic:      magic,
		LastEchoID: 0,
		MissCount:  0,
		LastSeen:   time.Now(),
	}

	g.timeWheel.Add(sessionID, state)
	g.logger.Debug("Added session to echo generator", "session_id", sessionID)
}

func (g *EchoGenerator) RemoveSession(sessionID uint16) {
	g.timeWheel.Remove(sessionID)
	g.logger.Debug("Removed session from echo generator", "session_id", sessionID)
}

func (g *EchoGenerator) HandleEchoReply(sessionID uint16, echoID uint8) {
	state := g.timeWheel.Get(sessionID)
	if state == nil {
		return
	}

	if state.LastEchoID == echoID {
		g.logger.Debug("Received LCP Echo-Reply",
			"session_id", sessionID,
			"echo_id", echoID)
		state.MissCount = 0
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

	for _, state := range sessions {
		if state.MissCount >= g.maxMisses {
			deadPeers = append(deadPeers, state.SessionID)
		} else {
			state.MissCount++
			state.LastEchoID++
			if g.sendEcho != nil {
				g.logger.Debug("Sending LCP Echo-Request",
					"session_id", state.SessionID,
					"echo_id", state.LastEchoID,
					"miss_count", state.MissCount)
				g.sendEcho(state.SessionID, state.LastEchoID)
			}
		}
	}

	for _, sessionID := range deadPeers {
		g.logger.Info("Dead peer detected", "session_id", sessionID)
		g.timeWheel.Remove(sessionID)
		if g.onDeadPeer != nil {
			g.onDeadPeer(sessionID)
		}
	}
}

func (g *EchoGenerator) SessionCount() int {
	return g.timeWheel.Count()
}
