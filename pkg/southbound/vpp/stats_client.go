package vpp

import (
	"fmt"
	"sync"

	"github.com/veesix-networks/osvbng/pkg/southbound"
	"go.fd.io/govpp/adapter/statsclient"
	"go.fd.io/govpp/api"
	"go.fd.io/govpp/core"
)

type StatsClient struct {
	client    *statsclient.StatsClient
	conn      *core.StatsConnection
	mu        sync.RWMutex
	connected bool
}

func NewStatsClient(socketPath string) *StatsClient {
	if socketPath == "" {
		socketPath = statsclient.DefaultSocketName
	}
	return &StatsClient{
		client: statsclient.NewStatsClient(socketPath),
	}
}

func (s *StatsClient) Connect() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.connected {
		return nil
	}

	conn, err := core.ConnectStats(s.client)
	if err != nil {
		return fmt.Errorf("connect to stats: %w", err)
	}

	s.conn = conn
	s.connected = true
	return nil
}

func (s *StatsClient) Disconnect() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn != nil {
		s.conn.Disconnect()
		s.conn = nil
	}
	s.connected = false
}

func (s *StatsClient) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connected
}

func (s *StatsClient) Reconnect() error {
	s.Disconnect()
	return s.Connect()
}

func (s *StatsClient) GetSystemStats() (*southbound.SystemStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.connected {
		return nil, fmt.Errorf("not connected to stats")
	}

	stats := new(api.SystemStats)
	if err := s.conn.GetSystemStats(stats); err != nil {
		return nil, fmt.Errorf("get system stats: %w", err)
	}

	return &southbound.SystemStats{
		VectorRate:          stats.VectorRate,
		InputRate:           stats.InputRate,
		LastUpdate:          stats.LastUpdate,
		LastStatsClear:      stats.LastStatsClear,
		Heartbeat:           stats.Heartbeat,
		NumWorkerThreads:    stats.NumWorkerThreads,
		VectorRatePerWorker: stats.VectorRatePerWorker,
	}, nil
}

func (s *StatsClient) GetMemoryStats() ([]southbound.MemoryStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.connected {
		return nil, fmt.Errorf("not connected to stats")
	}

	stats := new(api.MemoryStats)
	if err := s.conn.GetMemoryStats(stats); err != nil {
		return nil, fmt.Errorf("get memory stats: %w", err)
	}

	var result []southbound.MemoryStats

	for idx, counters := range stats.Main {
		result = append(result, southbound.MemoryStats{
			Heap:       fmt.Sprintf("main-%d", idx),
			Total:      counters.Total,
			Used:       counters.Used,
			Free:       counters.Free,
			UsedMMap:   counters.UsedMMap,
			TotalAlloc: counters.TotalAlloc,
			FreeChunks: counters.FreeChunks,
			Releasable: counters.Releasable,
		})
	}

	for idx, counters := range stats.Stat {
		result = append(result, southbound.MemoryStats{
			Heap:       fmt.Sprintf("stat-%d", idx),
			Total:      counters.Total,
			Used:       counters.Used,
			Free:       counters.Free,
			UsedMMap:   counters.UsedMMap,
			TotalAlloc: counters.TotalAlloc,
			FreeChunks: counters.FreeChunks,
			Releasable: counters.Releasable,
		})
	}

	return result, nil
}

func (s *StatsClient) GetInterfaceStats() ([]southbound.InterfaceStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.connected {
		return nil, fmt.Errorf("not connected to stats")
	}

	stats := new(api.InterfaceStats)
	if err := s.conn.GetInterfaceStats(stats); err != nil {
		return nil, fmt.Errorf("get interface stats: %w", err)
	}

	var result []southbound.InterfaceStats
	for _, iface := range stats.Interfaces {
		result = append(result, southbound.InterfaceStats{
			Name:     iface.InterfaceName,
			Index:    iface.InterfaceIndex,
			Rx:       iface.Rx.Packets,
			RxBytes:  iface.Rx.Bytes,
			RxErrors: iface.RxErrors,
			Tx:       iface.Tx.Packets,
			TxBytes:  iface.Tx.Bytes,
			TxErrors: iface.TxErrors,
			Drops:    iface.Drops,
			Punts:    iface.Punts,
		})
	}

	return result, nil
}

func (s *StatsClient) GetNodeStats() ([]southbound.NodeStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.connected {
		return nil, fmt.Errorf("not connected to stats")
	}

	stats := new(api.NodeStats)
	if err := s.conn.GetNodeStats(stats); err != nil {
		return nil, fmt.Errorf("get node stats: %w", err)
	}

	var result []southbound.NodeStats
	for _, node := range stats.Nodes {
		if node.Calls == 0 && node.Vectors == 0 {
			continue
		}
		result = append(result, southbound.NodeStats{
			Name:     node.NodeName,
			Index:    node.NodeIndex,
			Calls:    node.Calls,
			Vectors:  node.Vectors,
			Suspends: node.Suspends,
			Clocks:   node.Clocks,
		})
	}

	return result, nil
}

func (s *StatsClient) GetErrorStats() ([]southbound.ErrorStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.connected {
		return nil, fmt.Errorf("not connected to stats")
	}

	stats := new(api.ErrorStats)
	if err := s.conn.GetErrorStats(stats); err != nil {
		return nil, fmt.Errorf("get error stats: %w", err)
	}

	var result []southbound.ErrorStats
	for _, errStat := range stats.Errors {
		var total uint64
		for _, v := range errStat.Values {
			total += v
		}
		if total == 0 {
			continue
		}
		result = append(result, southbound.ErrorStats{
			Name:  errStat.CounterName,
			Count: total,
		})
	}

	return result, nil
}

func (s *StatsClient) GetBufferStats() ([]southbound.BufferStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.connected {
		return nil, fmt.Errorf("not connected to stats")
	}

	stats := new(api.BufferStats)
	if err := s.conn.GetBufferStats(stats); err != nil {
		return nil, fmt.Errorf("get buffer stats: %w", err)
	}

	var result []southbound.BufferStats
	for name, pool := range stats.Buffer {
		result = append(result, southbound.BufferStats{
			PoolName:  name,
			Cached:    pool.Cached,
			Used:      pool.Used,
			Available: pool.Available,
		})
	}

	return result, nil
}

func (s *StatsClient) GetAllStats() (*southbound.DataplaneStats, error) {
	result := &southbound.DataplaneStats{}

	if sys, err := s.GetSystemStats(); err == nil {
		result.System = sys
	}

	if mem, err := s.GetMemoryStats(); err == nil {
		result.Memory = mem
	}

	if ifaces, err := s.GetInterfaceStats(); err == nil {
		result.Interfaces = ifaces
	}

	if nodes, err := s.GetNodeStats(); err == nil {
		result.Nodes = nodes
	}

	if errs, err := s.GetErrorStats(); err == nil {
		result.Errors = errs
	}

	if bufs, err := s.GetBufferStats(); err == nil {
		result.Buffers = bufs
	}

	return result, nil
}
