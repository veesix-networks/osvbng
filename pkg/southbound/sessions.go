package southbound

import (
	"errors"
	"net"

	"github.com/veesix-networks/osvbng/pkg/config/qos"
)

// ErrSchedV2Unsupported reports a dataplane whose scheduler dump predates
// hierarchy and DRR state (QoS plugin API < 3.1.0), so aggregate membership
// cannot be answered over the wire.
var ErrSchedV2Unsupported = errors.New("dataplane does not support the scheduler v2 dump")

type Sessions interface {
	AddPPPoESession(sessionID uint16, clientIP net.IP, clientMAC net.HardwareAddr, localMAC net.HardwareAddr, encapIfIndex uint32, outerVLAN uint16, innerVLAN uint16, decapVrfID uint32, pppMTU uint16, policy MSSClampPolicy) (uint32, error)
	DeletePPPoESession(sessionID uint16, clientIP net.IP, clientMAC net.HardwareAddr) error
	AddPPPoESessionAsync(sessionID uint16, clientIP net.IP, clientMAC net.HardwareAddr, localMAC net.HardwareAddr, encapIfIndex uint32, outerVLAN uint16, innerVLAN uint16, decapVrfID uint32, pppMTU uint16, policy MSSClampPolicy, callback func(uint32, error))
	DeletePPPoESessionAsync(sessionID uint16, clientIP net.IP, clientMAC net.HardwareAddr, callback func(error))
	SetPPPoESessionLACTunneled(swIfIndex uint32, lacL2TPSessionIndex uint32, isLAC bool) error
	PPPoESetSessionIPv6(swIfIndex uint32, clientIP net.IP, isAdd bool) error
	PPPoESetDelegatedPrefix(swIfIndex uint32, prefix net.IPNet, nextHop net.IP, isAdd bool) error
	PPPoESetSessionIPv6Async(swIfIndex uint32, clientIP net.IP, isAdd bool, callback func(error))
	PPPoESetDelegatedPrefixAsync(swIfIndex uint32, prefix net.IPNet, nextHop net.IP, isAdd bool, callback func(error))

	AddIPoESession(clientMAC net.HardwareAddr, localMAC net.HardwareAddr, encapIfIndex uint32, outerVLAN uint16, innerVLAN uint16, decapVrfID uint32) (uint32, error)
	DeleteIPoESession(clientMAC net.HardwareAddr, encapIfIndex uint32, innerVLAN uint16) error
	IPoESetSessionIPv4(swIfIndex uint32, clientIP net.IP, isAdd bool) error
	IPoESetSessionIPv6(swIfIndex uint32, clientIP net.IP, isAdd bool) error
	IPoESetDelegatedPrefix(swIfIndex uint32, prefix net.IPNet, nextHop net.IP, isAdd bool) error

	AddIPoESessionAsync(clientMAC net.HardwareAddr, localMAC net.HardwareAddr, encapIfIndex uint32, outerVLAN uint16, innerVLAN uint16, decapVrfID uint32, callback func(uint32, error))
	DeleteIPoESessionAsync(clientMAC net.HardwareAddr, encapIfIndex uint32, innerVLAN uint16, callback func(error))
	IPoESetSessionIPv4Async(swIfIndex uint32, clientIP net.IP, isAdd bool, callback func(error))
	IPoESetSessionIPv6Async(swIfIndex uint32, clientIP net.IP, isAdd bool, callback func(error))
	IPoESetDelegatedPrefixAsync(swIfIndex uint32, prefix net.IPNet, nextHop net.IP, isAdd bool, callback func(error))

	IPoEEnableInput(ifaceName string) error

	ApplyQoS(swIfIndex uint32, ingress, egress *qos.Policy) error
	RemoveQoS(swIfIndex uint32) error

	ApplyScheduler(swIfIndex uint32, rateKbps uint32, cfg *qos.SchedulerConfig) error
	RemoveScheduler(swIfIndex uint32) error
	DumpSchedulers() ([]SchedulerState, error)
	// DumpScheduler reports one scheduler, nil when none exists on the
	// interface.
	DumpScheduler(swIfIndex uint32) (*SchedulerState, error)
	// DumpSchedulersByParent reports the members of one aggregate, addressed
	// by the (port sw_if_index, level, svlan_id) composite key. Returns
	// ErrSchedV2Unsupported against a dataplane without the v2 scheduler
	// dump, since membership is not on the v1 wire.
	DumpSchedulersByParent(parentSwIfIndex uint32, level string, svlanID uint16) ([]SchedulerState, error)

	// Aggregate shapers. A port aggregate and the S-VLAN aggregates beneath
	// it are both addressed by the port's sw_if_index; svlanID selects
	// between them, and is ignored at the port level.
	ApplyAggregate(swIfIndex uint32, cfg *qos.Aggregate) error
	UpdateAggregate(swIfIndex uint32, cfg *qos.Aggregate) error
	RemoveAggregate(swIfIndex uint32, level uint8, svlanID uint16) error
	DumpAggregates() ([]AggregateState, error)

	// L2TPv2 tunnels (the transport protocol). IDs are passed in HOST
	// byte order.
	AddL2TPTunnel(local, peer net.IP, localID, peerID, localPort, peerPort uint16, dfBit bool) (uint32, error)
	DeleteL2TPTunnel(local, peer net.IP, localID uint16) error

	// Subscriber sessions carried over L2TP.
	//   - PPPoL2TP: PPP terminating at the LNS (DECAP_IP). Creates a
	//     per-session vnet interface and binds the subscriber's IPs.
	//   - LAC raw: PPPoE-on-the-wire bridged out via L2TP (DECAP_RAW).
	//     No subscriber IP binding — the LNS owns the IP.
	// pppHdrSkip selects the data-frame PPP framing: 2 = HDLC
	// Address+Control prefix present (default; matches every major
	// LNS / pppd-based LAC), 0 = ACFC compressed. Resolved once from
	// operator config; the plugin stores it and reads it once per
	// packet, branch-free.
	AddPPPoL2TPSession(local, peer net.IP, localTunnelID, localSessionID, peerSessionID uint16, decapVrfID uint32, encapIfIndex uint32, pppHdrSkip uint8) (uint32, error)
	AddL2TPSessionRaw(local, peer net.IP, localTunnelID, localSessionID, peerSessionID uint16, rawNextNode string, rawOpaque uint32, encapIfIndex uint32, pppHdrSkip uint8) (uint32, error)
	DeleteL2TPSession(local, peer net.IP, localTunnelID, localSessionID uint16) error
	PPPoL2TPSetSubscriberIPv4(swIfIndex uint32, clientIP net.IP, isAdd bool) error
	PPPoL2TPSetSubscriberIPv6(swIfIndex uint32, clientIP net.IP, isAdd bool) error
	PPPoL2TPSetDelegatedPrefix(swIfIndex uint32, prefix net.IPNet, nextHop net.IP, isAdd bool) error
}

type SchedulerTinState struct {
	// Tin is a label so each flattened element lands in its own series;
	// without it all eight tins collapse into one label tuple.
	Tin         uint8  `json:"tin"           metric:"label"`
	Packets     uint64 `json:"packets"       metric:"name=qos.scheduler.tin.packets,type=counter,help=Packets through this QoS tin."`
	Bytes       uint64 `json:"bytes"         metric:"name=qos.scheduler.tin.bytes,type=counter,help=Bytes through this QoS tin."`
	Drops       uint64 `json:"drops"         metric:"name=qos.scheduler.tin.drops,type=counter,help=Packets dropped at this QoS tin."`
	ECNMarks    uint64 `json:"ecn_marks"     metric:"name=qos.scheduler.tin.ecn_marks,type=counter,help=ECN-marked packets at this QoS tin."`
	SparseFlows uint32 `json:"sparse_flows"  metric:"name=qos.scheduler.tin.sparse_flows,type=gauge,help=Sparse flows tracked at this QoS tin."`
	BulkFlows   uint32 `json:"bulk_flows"    metric:"name=qos.scheduler.tin.bulk_flows,type=gauge,help=Bulk flows tracked at this QoS tin."`
	FlowCount   uint32 `json:"flow_count"    metric:"name=qos.scheduler.tin.flow_count,type=gauge,help=Live flows tracked at this QoS tin. Zero on a v1-only dataplane."`
	PeakDelayUs uint32 `json:"peak_delay_us" metric:"name=qos.scheduler.tin.peak_delay_us,type=gauge,help=Peak packet sojourn delay at this QoS tin in microseconds. Reads zero until the dataplane computes it."`
	AvgDelayUs  uint32 `json:"avg_delay_us"  metric:"name=qos.scheduler.tin.avg_delay_us,type=gauge,help=Average packet sojourn delay at this QoS tin in microseconds. Reads zero until the dataplane computes it."`
}

// SchedulerState is one subscriber scheduler. The v2 dump adds the parent
// identity and DRR fields; against a v1-only dataplane they stay zero and
// HasParent is false. SessionID is best-effort enrichment resolved by the
// show layer, never by southbound, and is deliberately not a metric label
// (per-session label names are rejected by the telemetry registry).
type SchedulerState struct {
	SwIfIndex       uint32 `json:"sw_if_index"           metric:"label"`
	InterfaceName   string `json:"interface,omitempty"   metric:"label"`
	SessionID       string `json:"session_id,omitempty"`
	RateKbps        uint64 `json:"rate_kbps"             metric:"name=qos.scheduler.rate_kbps,type=gauge,help=QoS scheduler shaping rate in kbps."`
	TinMode         string `json:"tin_mode"              metric:"label"`
	TinCount        uint8  `json:"tin_count"             metric:"name=qos.scheduler.tin_count,type=gauge,help=Configured tin count on this QoS scheduler."`
	Weight          uint32 `json:"weight"                metric:"name=qos.scheduler.weight,type=gauge,help=Operator weight multiplier on this scheduler."`
	EffectiveWeight uint64 `json:"effective_weight"      metric:"name=qos.scheduler.effective_weight,type=gauge,help=Rate-derived DRR weight after the operator multiplier."`

	// Parent identity: the same (sw_if_index, level, svlan_id) composite key
	// AggregateState reports, so the two views join without a dataplane
	// index on the wire.
	HasParent       bool   `json:"has_parent"`
	ParentLevel     string `json:"parent_level,omitempty"`
	ParentSwIfIndex uint32 `json:"parent_sw_if_index,omitempty"`
	ParentSVLANID   uint16 `json:"parent_svlan_id,omitempty"`

	DRRActive     bool   `json:"drr_active"`
	DRRDeficit    int64  `json:"drr_deficit"`
	DRRBlocked    uint64 `json:"drr_blocked"    metric:"name=qos.scheduler.drr_blocked,type=counter,help=Dispatches deferred because siblings held the parent capacity."`
	ParentBlocked uint64 `json:"parent_blocked" metric:"name=qos.scheduler.parent_blocked,type=counter,help=Dispatches deferred because the parent chain was at its configured rate."`

	EnqueuedPkts  uint64 `json:"enqueued_pkts"  metric:"name=qos.scheduler.enqueued_packets,type=counter,help=Packets accepted into this scheduler."`
	EnqueuedBytes uint64 `json:"enqueued_bytes" metric:"name=qos.scheduler.enqueued_bytes,type=counter,help=Bytes accepted into this scheduler."`
	DequeuedPkts  uint64 `json:"dequeued_pkts"  metric:"name=qos.scheduler.dequeued_packets,type=counter,help=Packets transmitted by this scheduler."`
	DequeuedBytes uint64 `json:"dequeued_bytes" metric:"name=qos.scheduler.dequeued_bytes,type=counter,help=Bytes transmitted by this scheduler."`
	DroppedPkts   uint64 `json:"dropped_pkts"   metric:"name=qos.scheduler.dropped_packets,type=counter,help=Packets dropped by this scheduler across every drop path."`
	QueuedBuffers uint32 `json:"queued_buffers" metric:"name=qos.scheduler.queued_buffers,type=gauge,help=Packets currently held in this scheduler's queues."`

	BufferUsage uint32 `json:"buffer_usage" metric:"name=qos.scheduler.buffer_usage,type=gauge,help=Current scheduler buffer usage."`
	BufferLimit uint32 `json:"buffer_limit" metric:"name=qos.scheduler.buffer_limit,type=gauge,help=Scheduler buffer limit."`
	OwnerThread uint32 `json:"owner_thread"`

	// Config echoes, v2 dump only.
	OverheadBytes int16  `json:"overhead_bytes,omitempty"`
	ATMMode       string `json:"atm_mode,omitempty"`
	MPU           uint8  `json:"mpu,omitempty"`
	TargetUs      uint32 `json:"target_us,omitempty"`
	IntervalUs    uint32 `json:"interval_us,omitempty"`

	Tins []SchedulerTinState `json:"tins,omitempty" metric:"flatten"`
}

// AggregateState is one tier of the shaping hierarchy. A port and its
// S-VLANs both report against the port's sw_if_index, so Level and SVLANID
// are what distinguish them.
type AggregateState struct {
	SwIfIndex     uint32 `json:"sw_if_index"         metric:"label"`
	InterfaceName string `json:"interface,omitempty" metric:"label"`
	Level         string `json:"level"               metric:"label"`
	SVLANID       uint16 `json:"svlan_id"            metric:"label"`
	SVLANIDEnd    uint16 `json:"svlan_id_end"        metric:"label"`
	RateKbps      uint64 `json:"rate_kbps"           metric:"name=qos.aggregate.rate_kbps,type=gauge,help=Aggregate shaping rate in kbps."`
	Weight        uint32 `json:"weight"              metric:"name=qos.aggregate.weight,type=gauge,help=Operator weight multiplier on this aggregate."`
	// EffectiveWeight is this aggregate's own weight as a child of the tier
	// above: rate-derived, times the operator multiplier.
	EffectiveWeight uint64 `json:"effective_weight" metric:"name=qos.aggregate.effective_weight,type=gauge,help=Rate-derived DRR weight of this aggregate after the operator multiplier."`
	BurstMs         uint32 `json:"burst_ms"          metric:"name=qos.aggregate.burst_ms,type=gauge,help=Idle credit ceiling in milliseconds."`
	BufferUsage     uint32 `json:"buffer_usage"      metric:"name=qos.aggregate.buffer_usage,type=gauge,help=Current aggregate bytes queued."`
	BufferLimit     uint32 `json:"buffer_limit"      metric:"name=qos.aggregate.buffer_limit,type=gauge,help=Aggregate buffer limit in bytes."`

	// ActiveWeight is the sum of the effective weights of the children
	// currently competing for this tier, and is the denominator every child's
	// share is computed against. It moves with traffic, not configuration.
	ActiveWeight     uint64 `json:"active_weight"     metric:"name=qos.aggregate.active_weight,type=gauge,help=Sum of effective weights of active children."`
	ActiveChildren   uint32 `json:"active_children"   metric:"name=qos.aggregate.active_children,type=gauge,help=Children currently competing for this aggregate."`
	ShapedPkts       uint64 `json:"shaped_pkts"       metric:"name=qos.aggregate.shaped_packets,type=counter,help=Packets shaped through this aggregate."`
	ShapedBytes      uint64 `json:"shaped_bytes"      metric:"name=qos.aggregate.shaped_bytes,type=counter,help=Bytes shaped through this aggregate."`
	BackpressureEvts uint64 `json:"backpressure"      metric:"name=qos.aggregate.backpressure,type=counter,help=Packets dropped by aggregate buffer overflow."`

	// DRRBlocked and ParentBlocked mean different things to an operator:
	// the first is "your siblings are using the capacity", the second is
	// "this tier is at its own configured rate".
	DRRBlocked    uint64 `json:"drr_blocked"    metric:"name=qos.aggregate.drr_blocked,type=counter,help=Dispatches deferred because this tier had spent its share of the tier above."`
	ParentBlocked uint64 `json:"parent_blocked" metric:"name=qos.aggregate.parent_blocked,type=counter,help=Dispatches deferred because this tier was at its configured rate."`
}
