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
	Managed        bool   // M flag
	Other          bool   // O flag
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
