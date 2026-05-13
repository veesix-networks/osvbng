package southbound

import (
	"net"

	"github.com/veesix-networks/osvbng/pkg/config/qos"
)

type Sessions interface {
	AddPPPoESession(sessionID uint16, clientIP net.IP, clientMAC net.HardwareAddr, localMAC net.HardwareAddr, encapIfIndex uint32, outerVLAN uint16, innerVLAN uint16, decapVrfID uint32, pppMTU uint16, policy MSSClampPolicy) (uint32, error)
	DeletePPPoESession(sessionID uint16, clientIP net.IP, clientMAC net.HardwareAddr) error
	AddPPPoESessionAsync(sessionID uint16, clientIP net.IP, clientMAC net.HardwareAddr, localMAC net.HardwareAddr, encapIfIndex uint32, outerVLAN uint16, innerVLAN uint16, decapVrfID uint32, pppMTU uint16, policy MSSClampPolicy, callback func(uint32, error))
	DeletePPPoESessionAsync(sessionID uint16, clientIP net.IP, clientMAC net.HardwareAddr, callback func(error))
	SetPPPoESessionLACTunneled(swIfIndex uint32, lacL2TPSessionIndex uint32, isLAC bool) error

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

	// L2TPv2 tunnels and sessions. IDs are passed in HOST byte order.
	AddL2TPTunnel(local, peer net.IP, localID, peerID, localPort, peerPort uint16, dfBit bool) (uint32, error)
	DeleteL2TPTunnel(local, peer net.IP, localID uint16) error
	AddL2TPSessionIP(local, peer net.IP, localTunnelID, localSessionID, peerSessionID uint16, decapVrfID uint32, encapIfIndex uint32) (uint32, error)
	AddL2TPSessionRaw(local, peer net.IP, localTunnelID, localSessionID, peerSessionID uint16, rawNextNode string, rawOpaque uint32, encapIfIndex uint32) (uint32, error)
	DeleteL2TPSession(local, peer net.IP, localTunnelID, localSessionID uint16) error
}

type SchedulerTinState struct {
	Packets     uint64 `json:"packets"      metric:"name=qos.scheduler.tin.packets,type=counter,help=Packets through this QoS tin."`
	Drops       uint64 `json:"drops"        metric:"name=qos.scheduler.tin.drops,type=counter,help=Packets dropped at this QoS tin."`
	ECNMarks    uint64 `json:"ecn_marks"    metric:"name=qos.scheduler.tin.ecn_marks,type=counter,help=ECN-marked packets at this QoS tin."`
	SparseFlows uint32 `json:"sparse_flows" metric:"name=qos.scheduler.tin.sparse_flows,type=gauge,help=Sparse flows tracked at this QoS tin."`
	BulkFlows   uint32 `json:"bulk_flows"   metric:"name=qos.scheduler.tin.bulk_flows,type=gauge,help=Bulk flows tracked at this QoS tin."`
}

type SchedulerState struct {
	SwIfIndex   uint32              `json:"sw_if_index"     metric:"label"`
	RateKbps    uint64              `json:"rate_kbps"       metric:"name=qos.scheduler.rate_kbps,type=gauge,help=QoS scheduler shaping rate in kbps."`
	TinMode     string              `json:"tin_mode"        metric:"label"`
	TinCount    uint8               `json:"tin_count"       metric:"name=qos.scheduler.tin_count,type=gauge,help=Configured tin count on this QoS scheduler."`
	BufferUsage uint32              `json:"buffer_usage"    metric:"name=qos.scheduler.buffer_usage,type=gauge,help=Current scheduler buffer usage."`
	BufferLimit uint32              `json:"buffer_limit"    metric:"name=qos.scheduler.buffer_limit,type=gauge,help=Scheduler buffer limit."`
	Tins        []SchedulerTinState `json:"tins,omitempty"  metric:"flatten"`
}
