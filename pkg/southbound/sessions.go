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
}

type SchedulerTinState struct {
	Packets     uint64 `json:"packets"`
	Drops       uint64 `json:"drops"`
	ECNMarks    uint64 `json:"ecn_marks"`
	SparseFlows uint32 `json:"sparse_flows"`
	BulkFlows   uint32 `json:"bulk_flows"`
}

type SchedulerState struct {
	SwIfIndex   uint32              `json:"sw_if_index"`
	RateKbps    uint64              `json:"rate_kbps"`
	TinMode     string              `json:"tin_mode"`
	TinCount    uint8               `json:"tin_count"`
	BufferUsage uint32              `json:"buffer_usage"`
	BufferLimit uint32              `json:"buffer_limit"`
	Tins        []SchedulerTinState `json:"tins,omitempty"`
}
