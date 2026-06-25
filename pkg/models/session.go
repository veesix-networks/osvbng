package models

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

type SubscriberSession interface {
	GetSessionID() string
	GetAccessType() AccessType
	GetProtocol() Protocol
	GetState() SessionState
	GetMAC() net.HardwareAddr
	GetOuterVLAN() uint16
	GetInnerVLAN() uint16
	GetAAASessionID() string
	GetIPv4Address() net.IP
	GetIPv6Address() net.IP
	GetIPv6Prefix() string
	GetIfIndex() uint32
	GetVLANCount() int
	GetUsername() string
	GetServiceGroup() string
	GetSRGName() string
	GetActivatedAt() time.Time
}

type IPoESession struct {
	SessionID  string
	State      SessionState
	AccessType string
	Protocol   string

	MAC       net.HardwareAddr
	OuterVLAN uint16
	InnerVLAN uint16
	VLANCount int

	IfIndex         uint32
	AccessIfIndex   uint32
	AccessInterface string
	VRF             string
	ServiceGroup    string
	SRGName         string

	IPv4Address net.IP
	LeaseTime   uint32
	ClientID    []byte
	Hostname    string
	DHCPXID     uint32
	DHCPOptions map[uint8][]byte
	RelayInfo   map[uint8][]byte

	IPv6Address   net.IP
	IPv6Prefix    string
	IPv6LeaseTime uint32
	DUID          []byte

	Username string

	AAASessionID string

	ActivatedAt time.Time

	IPv4Pool   string
	IANAPool   string
	PDPool     string
	Attributes map[string]string
}

func (s *IPoESession) GetSessionID() string      { return s.SessionID }
func (s *IPoESession) GetAccessType() AccessType { return AccessTypeIPoE }
func (s *IPoESession) GetProtocol() Protocol     { return Protocol(s.Protocol) }
func (s *IPoESession) GetState() SessionState    { return s.State }
func (s *IPoESession) GetMAC() net.HardwareAddr  { return s.MAC }
func (s *IPoESession) GetOuterVLAN() uint16      { return s.OuterVLAN }
func (s *IPoESession) GetInnerVLAN() uint16      { return s.InnerVLAN }
func (s *IPoESession) GetAAASessionID() string   { return s.AAASessionID }
func (s *IPoESession) GetIPv4Address() net.IP    { return s.IPv4Address }
func (s *IPoESession) GetIPv6Address() net.IP    { return s.IPv6Address }
func (s *IPoESession) GetIPv6Prefix() string     { return s.IPv6Prefix }
func (s *IPoESession) GetIfIndex() uint32        { return s.IfIndex }
func (s *IPoESession) GetVLANCount() int         { return s.VLANCount }
func (s *IPoESession) GetUsername() string       { return s.Username }
func (s *IPoESession) GetServiceGroup() string   { return s.ServiceGroup }
func (s *IPoESession) GetSRGName() string        { return s.SRGName }
func (s *IPoESession) GetActivatedAt() time.Time { return s.ActivatedAt }

func (s *IPoESession) IsDualStack() bool {
	return s.IPv4Address != nil && (s.IPv6Address != nil || s.IPv6Prefix != "")
}

func (s *IPoESession) RedisKey() string {
	return fmt.Sprintf("osvbng:sessions:%s", s.SessionID)
}

func (s IPoESession) MarshalJSON() ([]byte, error) {
	type Alias IPoESession
	aux := &struct {
		MAC         string `json:"MAC,omitempty"`
		IPv4Address string `json:"IPv4Address,omitempty"`
		IPv6Address string `json:"IPv6Address,omitempty"`
		Alias
	}{
		Alias: (Alias)(s),
	}
	if s.MAC != nil {
		aux.MAC = s.MAC.String()
	}
	if s.IPv4Address != nil {
		aux.IPv4Address = s.IPv4Address.String()
	}
	if s.IPv6Address != nil {
		aux.IPv6Address = s.IPv6Address.String()
	}
	return json.Marshal(aux)
}

func (s *IPoESession) UnmarshalJSON(data []byte) error {
	type Alias IPoESession
	aux := &struct {
		MAC         string `json:"MAC"`
		IPv4Address string `json:"IPv4Address"`
		IPv6Address string `json:"IPv6Address"`
		*Alias
	}{
		Alias: (*Alias)(s),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if aux.MAC != "" {
		s.MAC, _ = net.ParseMAC(aux.MAC)
	}
	if aux.IPv4Address != "" && aux.IPv4Address != "<nil>" {
		s.IPv4Address = net.ParseIP(aux.IPv4Address)
	}
	if aux.IPv6Address != "" && aux.IPv6Address != "<nil>" {
		s.IPv6Address = net.ParseIP(aux.IPv6Address)
	}
	return nil
}

type PPPSession struct {
	SessionID    string
	State        SessionState
	AccessType   string
	Protocol     string
	PPPSessionID uint16

	MAC       net.HardwareAddr
	OuterVLAN uint16
	InnerVLAN uint16
	VLANCount int

	IfIndex         uint32
	AccessIfIndex   uint32
	AccessInterface string
	VRF             string
	ServiceGroup    string
	SRGName         string

	IPv4Address net.IP
	IPv6Address net.IP
	IPv6Prefix  string
	LCPState    string
	IPCPState   string
	IPv6CPState string

	// DHCPv6-over-PPP lease state, carried through the HA checkpoint so a
	// standby can release the provider lease and validate Renew after promotion.
	DUID          []byte
	IPv6LeaseTime uint32

	Username string

	AAASessionID string

	ActivatedAt time.Time

	IPv4Pool   string
	IANAPool   string
	LCPMagic   uint32
	Attributes map[string]string

	NegotiatedPPPMTU uint16
	IPv4MSS          uint16
	IPv6MSS          uint16

	// TunneledToLNS is the LNS IP this subscriber is currently bridged
	// to in LAC mode. Populated only when State == SessionStateTunneled;
	// nil for terminating PPPoE sessions.
	TunneledToLNS net.IP

	// L2TP is the LAC binding for this subscriber. Non-nil only when
	// the session is in `tunneled` state. Marshaled with `omitempty`
	// so non-LAC sessions render the same JSON as before.
	L2TP *L2TPBinding `json:"L2TP,omitempty"`
}

// L2TPBinding is the L2TPv2 session metadata for a LAC-bridged
// PPPoE subscriber. The PeerTunnelID/PeerSessionID are what the LNS
// sees as its local IDs.
type L2TPBinding struct {
	LocalTunnelID  uint16 `json:"LocalTunnelID"`
	PeerTunnelID   uint16 `json:"PeerTunnelID"`
	LocalSessionID uint16 `json:"LocalSessionID"`
	PeerSessionID  uint16 `json:"PeerSessionID"`
}

// L2TPTunnelSummary is the tunnel-level snapshot returned by
// `show l2tp tunnels`. One row per active tunnel; sessions are
// surfaced through the per-subscriber view.
type L2TPTunnelSummary struct {
	LocalIP       string    `json:"LocalIP"`
	PeerIP        string    `json:"PeerIP"`
	LocalID       uint16    `json:"LocalID"`
	PeerID        uint16    `json:"PeerID"`
	LocalHostname string    `json:"LocalHostname,omitempty"`
	PeerHostname  string    `json:"PeerHostname,omitempty"`
	Role          string    `json:"Role"`
	State         string    `json:"State"`
	SessionCount  int       `json:"SessionCount"`
	CreatedAt     time.Time `json:"CreatedAt"`
}

func (s *PPPSession) GetSessionID() string      { return s.SessionID }
func (s *PPPSession) GetAccessType() AccessType { return AccessTypePPPoE }
func (s *PPPSession) GetProtocol() Protocol     { return ProtocolPPPoESession }
func (s *PPPSession) GetState() SessionState    { return s.State }
func (s *PPPSession) GetMAC() net.HardwareAddr  { return s.MAC }
func (s *PPPSession) GetOuterVLAN() uint16      { return s.OuterVLAN }
func (s *PPPSession) GetInnerVLAN() uint16      { return s.InnerVLAN }
func (s *PPPSession) GetAAASessionID() string   { return s.AAASessionID }
func (s *PPPSession) GetIPv4Address() net.IP    { return s.IPv4Address }
func (s *PPPSession) GetIPv6Address() net.IP    { return s.IPv6Address }
func (s *PPPSession) GetIPv6Prefix() string     { return s.IPv6Prefix }
func (s *PPPSession) GetIfIndex() uint32        { return s.IfIndex }
func (s *PPPSession) GetVLANCount() int         { return s.VLANCount }
func (s *PPPSession) GetUsername() string       { return s.Username }
func (s *PPPSession) GetServiceGroup() string   { return s.ServiceGroup }
func (s *PPPSession) GetSRGName() string        { return s.SRGName }
func (s *PPPSession) GetActivatedAt() time.Time { return s.ActivatedAt }

func (s *PPPSession) RedisKey() string {
	return fmt.Sprintf("osvbng:sessions:%s", s.SessionID)
}

func MakeIPoESessionID(mac net.HardwareAddr, vlan uint16) string {
	return fmt.Sprintf("ipoe:%s:%d", mac.String(), vlan)
}

func MakePPPSessionID(sessionID uint16) string {
	return fmt.Sprintf("ppp:%d", sessionID)
}

// PPPoL2TPSession represents an LNS-terminated subscriber: PPP runs
// locally over L2TP transport. Subscriber identity comes from the LAC
// end of the tunnel; there is no MAC or VLAN at this layer.
type PPPoL2TPSession struct {
	SessionID    string
	State        SessionState
	AccessType   string
	Protocol     string
	AAASessionID string

	LocalIP            net.IP
	PeerIP             net.IP
	LocalTunnelID      uint16
	PeerTunnelID       uint16
	LocalSessionID     uint16
	PeerSessionID      uint16
	TunnelAssignmentID string
	LACHostname        string

	IfIndex      uint32
	VRF          string
	ServiceGroup string
	SRGName      string

	IPv4Address net.IP
	IPv6Address net.IP
	IPv6Prefix  string
	IPv4Pool    string
	IANAPool    string

	LCPState    string
	IPCPState   string
	IPv6CPState string
	LCPMagic    uint32

	Username string

	ActivatedAt time.Time
	Attributes  map[string]string

	NegotiatedPPPMTU uint16
	IPv4MSS          uint16
	IPv6MSS          uint16
}

func (s *PPPoL2TPSession) GetSessionID() string      { return s.SessionID }
func (s *PPPoL2TPSession) GetAccessType() AccessType { return AccessTypeL2TP }
func (s *PPPoL2TPSession) GetProtocol() Protocol     { return ProtocolL2TP }
func (s *PPPoL2TPSession) GetState() SessionState    { return s.State }
func (s *PPPoL2TPSession) GetMAC() net.HardwareAddr  { return nil }
func (s *PPPoL2TPSession) GetOuterVLAN() uint16      { return 0 }
func (s *PPPoL2TPSession) GetInnerVLAN() uint16      { return 0 }
func (s *PPPoL2TPSession) GetAAASessionID() string   { return s.AAASessionID }
func (s *PPPoL2TPSession) GetIPv4Address() net.IP    { return s.IPv4Address }
func (s *PPPoL2TPSession) GetIPv6Address() net.IP    { return s.IPv6Address }
func (s *PPPoL2TPSession) GetIPv6Prefix() string     { return s.IPv6Prefix }
func (s *PPPoL2TPSession) GetIfIndex() uint32        { return s.IfIndex }
func (s *PPPoL2TPSession) GetVLANCount() int         { return 0 }
func (s *PPPoL2TPSession) GetUsername() string       { return s.Username }
func (s *PPPoL2TPSession) GetServiceGroup() string   { return s.ServiceGroup }
func (s *PPPoL2TPSession) GetSRGName() string        { return s.SRGName }
func (s *PPPoL2TPSession) GetActivatedAt() time.Time { return s.ActivatedAt }

type SessionStats struct {
	RxPackets uint64 `json:"rx_packets"`
	RxBytes   uint64 `json:"rx_bytes"`
	TxPackets uint64 `json:"tx_packets"`
	TxBytes   uint64 `json:"tx_bytes"`
}

func (s *SessionStats) Marshal() ([]byte, error) {
	return json.Marshal(s)
}

func (s *SessionStats) Unmarshal(data []byte) error {
	return json.Unmarshal(data, s)
}
