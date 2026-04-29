package southbound

import "net"

type InterfaceInfo struct {
	SwIfIndex    uint32
	Name         string
	AdminUp      bool
	LinkUp       bool
	MTU          uint32
	OuterVlanID  uint16
	InnerVlanID  uint16
	SupSwIfIndex uint32
}

type IPAddressInfo struct {
	SwIfIndex uint32
	Address   string
	IsIPv6    bool
}

type UnnumberedInfo struct {
	SwIfIndex   uint32
	IPSwIfIndex uint32
}

type IPv6RAConfig struct {
	Managed        bool // M flag
	Other          bool // O flag
	RouterLifetime uint32
	MaxInterval    uint32
	MinInterval    uint32
}

type IPv6RAPrefixConfig struct {
	Prefix            string
	OnLink            bool // L flag
	Autonomous        bool // A flag
	ValidLifetime     uint32
	PreferredLifetime uint32
}

type IPv6RAInfo struct {
	SwIfIndex          uint32
	Managed            bool
	Other              bool
	RouterLifetimeSecs uint16
	MaxIntervalSecs    float64
	MinIntervalSecs    float64
	SendRadv           bool
}

type PuntRegistration struct {
	SwIfIndex uint32
	Protocol  uint8
}

type PuntStats struct {
	Protocol       uint8
	PacketsPunted  uint64
	PacketsDropped uint64
	PacketsPoliced uint64
	PolicerRate    float64
	PolicerBurst   uint32
}

type MrouteInfo struct {
	TableID    uint32
	GrpAddress net.IP
	SrcAddress net.IP
	IsIPv6     bool
}

type IPTableInfo struct {
	TableID uint32 `json:"table_id"`
	Name    string `json:"name"`
	IsIPv6  bool   `json:"is_ipv6"`
}

type IPMTableInfo struct {
	TableID uint32
	Name    string
	IsIPv6  bool
}

type MPLSTableInfo struct {
	TableID uint32
	Name    string
}

type MPLSRouteEntry struct {
	Label       uint32          `json:"label"`
	Eos         bool            `json:"eos"`
	EosProto    uint8           `json:"eos_proto"`
	IsMulticast bool            `json:"is_multicast"`
	Paths       []MPLSRoutePath `json:"paths"`
}

type MPLSRoutePath struct {
	SwIfIndex  uint32   `json:"sw_if_index"`
	Interface  string   `json:"interface,omitempty"`
	NextHop    string   `json:"next_hop,omitempty"`
	Weight     uint8    `json:"weight"`
	Preference uint8    `json:"preference"`
	Labels     []uint32 `json:"labels,omitempty"`
}

type MPLSInterfaceInfo struct {
	SwIfIndex uint32 `json:"sw_if_index"`
	Name      string `json:"name,omitempty"`
}

type SystemStats struct {
	VectorRate          uint64   `json:"vector_rate"        metric:"name=system.vector_rate,type=gauge,help=VPP vector rate."`
	InputRate           uint64   `json:"input_rate"         metric:"name=system.input_rate,type=gauge,help=VPP input rate."`
	LastUpdate          uint64   `json:"last_update"        metric:"name=system.last_update,type=gauge,help=VPP last stats update timestamp."`
	LastStatsClear      uint64   `json:"last_stats_clear"   metric:"name=system.last_stats_clear,type=gauge,help=VPP last stats clear timestamp."`
	Heartbeat           uint64   `json:"heartbeat"          metric:"name=system.heartbeat,type=counter,help=VPP heartbeat counter."`
	NumWorkerThreads    uint64   `json:"num_worker_threads" metric:"name=system.worker_threads,type=gauge,help=VPP worker thread count."`
	VectorRatePerWorker []uint64 `json:"vector_rate_per_worker"`
}

type MemoryStats struct {
	Heap       string `json:"heap"        metric:"label"`
	Total      uint64 `json:"total"       metric:"name=memory.total_bytes,type=gauge,help=VPP heap total bytes."`
	Used       uint64 `json:"used"        metric:"name=memory.used_bytes,type=gauge,help=VPP heap used bytes."`
	Free       uint64 `json:"free"        metric:"name=memory.free_bytes,type=gauge,help=VPP heap free bytes."`
	UsedMMap   uint64 `json:"used_mmap"   metric:"name=memory.used_mmap_bytes,type=gauge,help=VPP heap used mmap bytes."`
	TotalAlloc uint64 `json:"total_alloc" metric:"name=memory.total_alloc_bytes,type=counter,help=VPP heap total allocated bytes."`
	FreeChunks uint64 `json:"free_chunks" metric:"name=memory.free_chunks,type=gauge,help=VPP heap free chunks."`
	Releasable uint64 `json:"releasable"  metric:"name=memory.releasable_bytes,type=gauge,help=VPP heap releasable bytes."`
}

type InterfaceStats struct {
	Name     string `json:"name"     metric:"label"`
	Index    uint32 `json:"index"    metric:"label"`
	Rx       uint64 `json:"rx_packets" metric:"name=interface.rx_packets,type=counter,help=VPP per-interface received packets."`
	RxBytes  uint64 `json:"rx_bytes"   metric:"name=interface.rx_bytes,type=counter,help=VPP per-interface received bytes."`
	RxErrors uint64 `json:"rx_errors"  metric:"name=interface.rx_errors,type=counter,help=VPP per-interface receive errors."`
	Tx       uint64 `json:"tx_packets" metric:"name=interface.tx_packets,type=counter,help=VPP per-interface transmitted packets."`
	TxBytes  uint64 `json:"tx_bytes"   metric:"name=interface.tx_bytes,type=counter,help=VPP per-interface transmitted bytes."`
	TxErrors uint64 `json:"tx_errors"  metric:"name=interface.tx_errors,type=counter,help=VPP per-interface transmit errors."`
	Drops    uint64 `json:"drops"      metric:"name=interface.drops,type=counter,help=VPP per-interface dropped packets."`
	Punts    uint64 `json:"punts"      metric:"name=interface.punts,type=counter,help=VPP per-interface punted packets."`
}

type NodeStats struct {
	Name     string `json:"name"     metric:"label"`
	Index    uint32 `json:"index"    metric:"label"`
	Calls    uint64 `json:"calls"    metric:"name=node.calls,type=counter,help=VPP graph node calls."`
	Vectors  uint64 `json:"vectors"  metric:"name=node.vectors,type=counter,help=VPP graph node vectors processed."`
	Suspends uint64 `json:"suspends" metric:"name=node.suspends,type=counter,help=VPP graph node suspends."`
	Clocks   uint64 `json:"clocks"   metric:"name=node.clocks,type=counter,help=VPP graph node clock cycles."`
}

type ErrorStats struct {
	Name  string `json:"name"  metric:"label"`
	Count uint64 `json:"count" metric:"name=error.count,type=counter,help=VPP error counter."`
}

type BufferStats struct {
	PoolName  string  `json:"pool_name" metric:"label"`
	Cached    float64 `json:"cached"    metric:"name=buffer.cached,type=gauge,help=VPP cached buffers."`
	Used      float64 `json:"used"      metric:"name=buffer.used,type=gauge,help=VPP used buffers."`
	Available float64 `json:"available" metric:"name=buffer.available,type=gauge,help=VPP available buffers."`
}

type DataplaneStats struct {
	System     *SystemStats     `json:"system,omitempty"`
	Memory     []MemoryStats    `json:"memory,omitempty"`
	Interfaces []InterfaceStats `json:"interfaces,omitempty"`
	Nodes      []NodeStats      `json:"nodes,omitempty"`
	Errors     []ErrorStats     `json:"errors,omitempty"`
	Buffers    []BufferStats    `json:"buffers,omitempty"`
}
