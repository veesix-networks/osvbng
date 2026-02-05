package srg

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/redis/go-redis/v9"
)

type Member struct {
	ID            string
	LastHeartbeat time.Time
	Priority      int
	Status        string
}

type Manager struct {
	localBNGID  string
	priority    int
	virtualMAC  net.HardwareAddr
	ring        *HashRing
	liveMembers map[string]*Member
	mu          sync.RWMutex

	redis  *redis.Client
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	heartbeatInterval time.Duration
	memberTimeout     time.Duration
	vnodeCount        int
	logger            *slog.Logger
}

type Config struct {
	BNGID             string
	Priority          int
	VirtualMAC        string
	RedisAddr         string
	HeartbeatInterval time.Duration
	MemberTimeout     time.Duration
	VNodeCount        int
}

func NewManager(cfg Config) (*Manager, error) {
	vmac, err := net.ParseMAC(cfg.VirtualMAC)
	if err != nil {
		return nil, fmt.Errorf("invalid virtual MAC: %w", err)
	}

	client := redis.NewClient(&redis.Options{
		Addr: cfg.RedisAddr,
	})

	ctx, cancel := context.WithCancel(context.Background())

	if err := client.Ping(ctx).Err(); err != nil {
		cancel()
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	vnodeCount := cfg.VNodeCount
	if vnodeCount == 0 {
		vnodeCount = 150
	}

	m := &Manager{
		localBNGID:        cfg.BNGID,
		priority:          cfg.Priority,
		virtualMAC:        vmac,
		liveMembers:       make(map[string]*Member),
		ring:              NewHashRing([]string{cfg.BNGID}, vnodeCount),
		redis:             client,
		ctx:               ctx,
		cancel:            cancel,
		heartbeatInterval: cfg.HeartbeatInterval,
		memberTimeout:     cfg.MemberTimeout,
		vnodeCount:        vnodeCount,
		logger:            logger.Get(logger.SRG),
	}

	return m, nil
}

func (m *Manager) Start() {
	m.wg.Add(2)
	go m.publishHeartbeats()
	go m.monitorMembers()
}

func (m *Manager) Stop() {
	m.cancel()
	m.wg.Wait()
	m.redis.Close()
}

func (m *Manager) IsDF(svlan uint16, mac string, cvlan uint16) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := fmt.Sprintf("%s:%d", mac, cvlan)
	elected := m.ring.Get(key)
	return elected == m.localBNGID
}

func (m *Manager) GetVirtualMAC(svlan uint16) net.HardwareAddr {
	return m.virtualMAC
}

func (m *Manager) GetGroupForSVLAN(svlan uint16) string {
	return "default"
}

func (m *Manager) publishHeartbeats() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			key := fmt.Sprintf("osvbng:srg:members:%s", m.localBNGID)
			err := m.redis.HSet(m.ctx, key,
				"bng_id", m.localBNGID,
				"last_heartbeat", time.Now().Unix(),
				"status", "active",
				"priority", m.priority,
			).Err()

			if err != nil {
				m.logger.Warn("Failed to publish heartbeat", "error", err)
				continue
			}

			m.redis.Expire(m.ctx, key, m.memberTimeout)
		}
	}
}

func (m *Manager) monitorMembers() {
	defer m.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			members := m.scanMembers()

			if m.membershipChanged(members) {
				m.logger.Info("Membership changed", "members", members)
				m.rebuildRing(members)
			}
		}
	}
}

func (m *Manager) scanMembers() []string {
	pattern := "osvbng:srg:members:*"
	var cursor uint64
	members := []string{}

	for {
		keys, nextCursor, err := m.redis.Scan(m.ctx, cursor, pattern, 100).Result()
		if err != nil {
			m.logger.Warn("Failed to scan members", "error", err)
			break
		}

		for _, key := range keys {
			parts := strings.Split(key, ":")
			if len(parts) > 0 {
				bngID := parts[len(parts)-1]
				members = append(members, bngID)
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return members
}

func (m *Manager) membershipChanged(newMembers []string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(newMembers) != len(m.liveMembers) {
		return true
	}

	for _, id := range newMembers {
		if _, exists := m.liveMembers[id]; !exists {
			return true
		}
	}

	return false
}

func (m *Manager) rebuildRing(members []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ring = NewHashRing(members, m.vnodeCount)

	m.liveMembers = make(map[string]*Member)
	for _, id := range members {
		m.liveMembers[id] = &Member{
			ID:            id,
			LastHeartbeat: time.Now(),
			Status:        "active",
		}
	}

	m.logger.Info("Ring rebuilt", "member_count", len(members))
}
