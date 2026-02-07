package southbound

import (
	"fmt"
	"sync"

	"go.fd.io/govpp/adapter/statsclient"
	"go.fd.io/govpp/api"
	"go.fd.io/govpp/core"
)

type SystemStats struct {
	VectorRate          uint64   `json:"vector_rate" prometheus:"name=vpp_system_vector_rate,help=Vector rate,type=gauge"`
	InputRate           uint64   `json:"input_rate" prometheus:"name=vpp_system_input_rate,help=Input rate,type=gauge"`
	LastUpdate          uint64   `json:"last_update" prometheus:"name=vpp_system_last_update,help=Last update timestamp,type=gauge"`
	LastStatsClear      uint64   `json:"last_stats_clear" prometheus:"name=vpp_system_last_stats_clear,help=Last stats clear timestamp,type=gauge"`
	Heartbeat           uint64   `json:"heartbeat" prometheus:"name=vpp_system_heartbeat,help=Heartbeat,type=counter"`
	NumWorkerThreads    uint64   `json:"num_worker_threads" prometheus:"name=vpp_system_worker_threads,help=Number of worker threads,type=gauge"`
	VectorRatePerWorker []uint64 `json:"vector_rate_per_worker"`
}

type MemoryStats struct {
	Heap       string `json:"heap" prometheus:"label"`
	Total      uint64 `json:"total" prometheus:"name=vpp_memory_total_bytes,help=Total memory bytes,type=gauge"`
	Used       uint64 `json:"used" prometheus:"name=vpp_memory_used_bytes,help=Used memory bytes,type=gauge"`
	Free       uint64 `json:"free" prometheus:"name=vpp_memory_free_bytes,help=Free memory bytes,type=gauge"`
	UsedMMap   uint64 `json:"used_mmap" prometheus:"name=vpp_memory_used_mmap_bytes,help=Used mmap bytes,type=gauge"`
	TotalAlloc uint64 `json:"total_alloc" prometheus:"name=vpp_memory_total_alloc_bytes,help=Total allocated bytes,type=counter"`
	FreeChunks uint64 `json:"free_chunks" prometheus:"name=vpp_memory_free_chunks,help=Free chunks,type=gauge"`
	Releasable uint64 `json:"releasable" prometheus:"name=vpp_memory_releasable_bytes,help=Releasable bytes,type=gauge"`
}

type InterfaceStats struct {
	Name      string `json:"name" prometheus:"label"`
	Index     uint32 `json:"index" prometheus:"label"`
	Rx        uint64 `json:"rx_packets" prometheus:"name=vpp_interface_rx_packets,help=Received packets,type=counter"`
	RxBytes   uint64 `json:"rx_bytes" prometheus:"name=vpp_interface_rx_bytes,help=Received bytes,type=counter"`
	RxErrors  uint64 `json:"rx_errors" prometheus:"name=vpp_interface_rx_errors,help=Receive errors,type=counter"`
	Tx        uint64 `json:"tx_packets" prometheus:"name=vpp_interface_tx_packets,help=Transmitted packets,type=counter"`
	TxBytes   uint64 `json:"tx_bytes" prometheus:"name=vpp_interface_tx_bytes,help=Transmitted bytes,type=counter"`
	TxErrors  uint64 `json:"tx_errors" prometheus:"name=vpp_interface_tx_errors,help=Transmit errors,type=counter"`
	Drops     uint64 `json:"drops" prometheus:"name=vpp_interface_drops,help=Dropped packets,type=counter"`
	Punts     uint64 `json:"punts" prometheus:"name=vpp_interface_punts,help=Punted packets,type=counter"`
}

type NodeStats struct {
	Name     string `json:"name" prometheus:"label"`
	Index    uint32 `json:"index" prometheus:"label"`
	Calls    uint64 `json:"calls" prometheus:"name=vpp_node_calls,help=Number of calls,type=counter"`
	Vectors  uint64 `json:"vectors" prometheus:"name=vpp_node_vectors,help=Number of vectors,type=counter"`
	Suspends uint64 `json:"suspends" prometheus:"name=vpp_node_suspends,help=Number of suspends,type=counter"`
	Clocks   uint64 `json:"clocks" prometheus:"name=vpp_node_clocks,help=Clock cycles,type=counter"`
}

type ErrorStats struct {
	Name  string `json:"name" prometheus:"label"`
	Count uint64 `json:"count" prometheus:"name=vpp_error_count,help=Error count,type=counter"`
}

type BufferStats struct {
	PoolName  string  `json:"pool_name" prometheus:"label"`
	Cached    float64 `json:"cached" prometheus:"name=vpp_buffer_cached,help=Cached buffers,type=gauge"`
	Used      float64 `json:"used" prometheus:"name=vpp_buffer_used,help=Used buffers,type=gauge"`
	Available float64 `json:"available" prometheus:"name=vpp_buffer_available,help=Available buffers,type=gauge"`
}

type DataplaneStats struct {
	System     *SystemStats     `json:"system,omitempty"`
	Memory     []MemoryStats    `json:"memory,omitempty"`
	Interfaces []InterfaceStats `json:"interfaces,omitempty"`
	Nodes      []NodeStats      `json:"nodes,omitempty"`
	Errors     []ErrorStats     `json:"errors,omitempty"`
	Buffers    []BufferStats    `json:"buffers,omitempty"`
}

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

func (s *StatsClient) GetSystemStats() (*SystemStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.connected {
		return nil, fmt.Errorf("not connected to stats")
	}

	stats := new(api.SystemStats)
	if err := s.conn.GetSystemStats(stats); err != nil {
		return nil, fmt.Errorf("get system stats: %w", err)
	}

	return &SystemStats{
		VectorRate:          stats.VectorRate,
		InputRate:           stats.InputRate,
		LastUpdate:          stats.LastUpdate,
		LastStatsClear:      stats.LastStatsClear,
		Heartbeat:           stats.Heartbeat,
		NumWorkerThreads:    stats.NumWorkerThreads,
		VectorRatePerWorker: stats.VectorRatePerWorker,
	}, nil
}

func (s *StatsClient) GetMemoryStats() ([]MemoryStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.connected {
		return nil, fmt.Errorf("not connected to stats")
	}

	stats := new(api.MemoryStats)
	if err := s.conn.GetMemoryStats(stats); err != nil {
		return nil, fmt.Errorf("get memory stats: %w", err)
	}

	var result []MemoryStats

	for idx, counters := range stats.Main {
		result = append(result, MemoryStats{
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
		result = append(result, MemoryStats{
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

func (s *StatsClient) GetInterfaceStats() ([]InterfaceStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.connected {
		return nil, fmt.Errorf("not connected to stats")
	}

	stats := new(api.InterfaceStats)
	if err := s.conn.GetInterfaceStats(stats); err != nil {
		return nil, fmt.Errorf("get interface stats: %w", err)
	}

	var result []InterfaceStats
	for _, iface := range stats.Interfaces {
		result = append(result, InterfaceStats{
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

func (s *StatsClient) GetNodeStats() ([]NodeStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.connected {
		return nil, fmt.Errorf("not connected to stats")
	}

	stats := new(api.NodeStats)
	if err := s.conn.GetNodeStats(stats); err != nil {
		return nil, fmt.Errorf("get node stats: %w", err)
	}

	var result []NodeStats
	for _, node := range stats.Nodes {
		if node.Calls == 0 && node.Vectors == 0 {
			continue
		}
		result = append(result, NodeStats{
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

func (s *StatsClient) GetErrorStats() ([]ErrorStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.connected {
		return nil, fmt.Errorf("not connected to stats")
	}

	stats := new(api.ErrorStats)
	if err := s.conn.GetErrorStats(stats); err != nil {
		return nil, fmt.Errorf("get error stats: %w", err)
	}

	var result []ErrorStats
	for _, errStat := range stats.Errors {
		var total uint64
		for _, v := range errStat.Values {
			total += v
		}
		if total == 0 {
			continue
		}
		result = append(result, ErrorStats{
			Name:  errStat.CounterName,
			Count: total,
		})
	}

	return result, nil
}

func (s *StatsClient) GetBufferStats() ([]BufferStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.connected {
		return nil, fmt.Errorf("not connected to stats")
	}

	stats := new(api.BufferStats)
	if err := s.conn.GetBufferStats(stats); err != nil {
		return nil, fmt.Errorf("get buffer stats: %w", err)
	}

	var result []BufferStats
	for name, pool := range stats.Buffer {
		result = append(result, BufferStats{
			PoolName:  name,
			Cached:    pool.Cached,
			Used:      pool.Used,
			Available: pool.Available,
		})
	}

	return result, nil
}

func (s *StatsClient) GetAllStats() (*DataplaneStats, error) {
	result := &DataplaneStats{}

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
