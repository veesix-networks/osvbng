package models

import (
	"encoding/json"
	"time"
)

type Event struct {
	ID         string     `json:"event_id"`
	Type       EventType  `json:"event_type"`
	Timestamp  time.Time  `json:"timestamp"`
	AccessType AccessType `json:"access_type"`
	Protocol   Protocol   `json:"protocol"`
	SessionID  string     `json:"session_id,omitempty"`

	Payload json.RawMessage `json:"payload,omitempty"`
}

func (e *Event) GetPayload(v interface{}) error {
	if len(e.Payload) == 0 {
		return nil
	}
	return json.Unmarshal(e.Payload, v)
}

func (e *Event) SetPayload(v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	e.Payload = data
	return nil
}

type AAARequest struct {
	RequestID     string            `json:"request_id"`
	Username      string            `json:"username"`
	MAC           string            `json:"mac"`
	AcctSessionID string            `json:"acct_session_id"`
	SVLAN         uint16            `json:"svlan"`
	CVLAN         uint16            `json:"cvlan"`
	Interface     string            `json:"interface"`
	PolicyName    string            `json:"policy_name"`
	Attributes    map[string]string `json:"attributes,omitempty"`
}

type AAAResponse struct {
	RequestID  string                 `json:"request_id"`
	Allowed    bool                   `json:"allowed"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
	Error      string                 `json:"error,omitempty"`
}

type EgressPacketPayload struct {
	RawData   []byte `json:"raw_data"`
	DstMAC    string `json:"dst_mac"`
	SrcMAC    string `json:"src_mac"`
	OuterVLAN uint16 `json:"outer_vlan"`
	InnerVLAN uint16 `json:"inner_vlan"`
	SwIfIndex uint32 `json:"sw_if_index"`
}

type EventType string

const (
	EventTypePacket           EventType = "packet"
	EventTypeSessionLifecycle EventType = "session_lifecycle"
	EventTypeAAARequest       EventType = "aaa_request"
	EventTypeAAAResponse      EventType = "aaa_response"
	EventTypeEgress           EventType = "egress"
)

type AccessType string

const (
	AccessTypeIPoE    AccessType = "ipoe"
	AccessTypePPPoE   AccessType = "pppoe"
	AccessTypeL2TP    AccessType = "l2tp"
	AccessTypeUnknown AccessType = "unknown"
)

type Protocol string

const (
	ProtocolDHCPv4         Protocol = "dhcpv4"
	ProtocolDHCPv6         Protocol = "dhcpv6"
	ProtocolARP            Protocol = "arp"
	ProtocolPPPoEDiscovery Protocol = "pppoe_discovery"
	ProtocolPPPoESession   Protocol = "pppoe_session"
	ProtocolIPv6ND         Protocol = "ipv6_nd"
	ProtocolPPP            Protocol = "ppp"
	ProtocolL2TP           Protocol = "l2tp"
	ProtocolUnknown        Protocol = "unknown"
)

type SessionState string

const (
	SessionStateUnknown     SessionState = "unknown"
	SessionStateDiscovering SessionState = "discovering"
	SessionStateOffered     SessionState = "offered"
	SessionStateRequesting  SessionState = "requesting"
	SessionStateActive      SessionState = "active"
	SessionStateReleased    SessionState = "released"
)
