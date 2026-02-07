package cppm

import (
	"sync"
	"time"
)

type Protocol string

const (
	ProtocolDHCPv4 Protocol = "dhcpv4"
	ProtocolDHCPv6 Protocol = "dhcpv6"
	ProtocolARP    Protocol = "arp"
	ProtocolPPPoE  Protocol = "pppoe"
	ProtocolIPv6ND Protocol = "ipv6nd"
	ProtocolL2TP   Protocol = "l2tp"
)

type PolicerConfig struct {
	Rate  float64
	Burst int
}

type Config struct {
	Policers map[Protocol]PolicerConfig
}

func DefaultConfig() Config {
	return Config{
		Policers: map[Protocol]PolicerConfig{
			ProtocolDHCPv4: {Rate: 1000, Burst: 100},
			ProtocolDHCPv6: {Rate: 1000, Burst: 100},
			ProtocolARP:    {Rate: 500, Burst: 50},
			ProtocolPPPoE:  {Rate: 1000, Burst: 100},
			ProtocolIPv6ND: {Rate: 500, Burst: 50},
			ProtocolL2TP:   {Rate: 500, Burst: 50},
		},
	}
}

type Policer struct {
	rate       float64
	burst      int
	tokens     float64
	lastUpdate time.Time
	allowed    uint64
	policed    uint64
	mu         sync.Mutex
}

func NewPolicer(rate float64, burst int) *Policer {
	return &Policer{
		rate:       rate,
		burst:      burst,
		tokens:     float64(burst),
		lastUpdate: time.Now(),
	}
}

func (p *Policer) Allow() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(p.lastUpdate).Seconds()
	p.lastUpdate = now

	p.tokens += elapsed * p.rate
	if p.tokens > float64(p.burst) {
		p.tokens = float64(p.burst)
	}

	if p.tokens >= 1 {
		p.tokens--
		p.allowed++
		return true
	}

	p.policed++
	return false
}

func (p *Policer) Rate() float64 {
	return p.rate
}

func (p *Policer) Burst() int {
	return p.burst
}

type Manager struct {
	policers map[Protocol]*Policer
	mu       sync.RWMutex
}

func NewManager(cfg Config) *Manager {
	m := &Manager{
		policers: make(map[Protocol]*Policer),
	}

	for proto, pc := range cfg.Policers {
		m.policers[proto] = NewPolicer(pc.Rate, pc.Burst)
	}

	return m
}

func (m *Manager) Get(proto Protocol) *Policer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.policers[proto]
}

func (m *Manager) Allow(proto Protocol) bool {
	p := m.Get(proto)
	if p == nil {
		return true
	}
	return p.Allow()
}

type Stats struct {
	Protocol string  `json:"protocol"`
	Rate     float64 `json:"rate"`
	Burst    int     `json:"burst"`
	Allowed  uint64  `json:"allowed"`
	Policed  uint64  `json:"policed"`
}

func (m *Manager) GetStats() []Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var stats []Stats
	for proto, p := range m.policers {
		stats = append(stats, Stats{
			Protocol: string(proto),
			Rate:     p.rate,
			Burst:    p.burst,
			Allowed:  p.allowed,
			Policed:  p.policed,
		})
	}
	return stats
}

func (m *Manager) Configure(proto Protocol, rate float64, burst int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if p, ok := m.policers[proto]; ok {
		p.mu.Lock()
		p.rate = rate
		p.burst = burst
		p.tokens = float64(burst)
		p.mu.Unlock()
	} else {
		m.policers[proto] = NewPolicer(rate, burst)
	}
}
