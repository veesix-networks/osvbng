package models

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
	OuterTPID uint16 `json:"outer_tpid,omitempty"`
	SwIfIndex uint32 `json:"sw_if_index"`
}

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
