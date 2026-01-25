package ppp

const (
	ProtoLCP    uint16 = 0xc021
	ProtoPAP    uint16 = 0xc023
	ProtoLQR    uint16 = 0xc025
	ProtoCHAP   uint16 = 0xc223
	ProtoIPCP   uint16 = 0x8021
	ProtoIPv6CP uint16 = 0x8057
	ProtoIP     uint16 = 0x0021
	ProtoIPv6   uint16 = 0x0057
)

const (
	ConfReq  uint8 = 1
	ConfAck  uint8 = 2
	ConfNak  uint8 = 3
	ConfRej  uint8 = 4
	TermReq  uint8 = 5
	TermAck  uint8 = 6
	CodeRej  uint8 = 7
	ProtoRej uint8 = 8
	EchoReq  uint8 = 9
	EchoRep  uint8 = 10
	DiscReq  uint8 = 11
)

const (
	LCPOptMRU       uint8 = 1
	LCPOptAuthProto uint8 = 3
	LCPOptQuality   uint8 = 4
	LCPOptMagic     uint8 = 5
	LCPOptPFC       uint8 = 7
	LCPOptACFC      uint8 = 8
)

const (
	IPCPOptAddresses     uint8 = 1
	IPCPOptCompression   uint8 = 2
	IPCPOptAddress       uint8 = 3
	IPCPOptPrimaryDNS    uint8 = 129
	IPCPOptPrimaryNBNS   uint8 = 130
	IPCPOptSecondaryDNS  uint8 = 131
	IPCPOptSecondaryNBNS uint8 = 132
)

const (
	IPv6CPOptInterfaceID uint8 = 1
)

const (
	CHAPMD5  uint8 = 5
	CHAPMSv1 uint8 = 128
	CHAPMSv2 uint8 = 129
)

const (
	PAPAuthReq uint8 = 1
	PAPAuthAck uint8 = 2
	PAPAuthNak uint8 = 3
)

const (
	CHAPChallenge uint8 = 1
	CHAPResponse  uint8 = 2
	CHAPSuccess   uint8 = 3
	CHAPFailure   uint8 = 4
)

const (
	DefaultMRU      uint16 = 1500
	DefaultPPPoEMRU uint16 = 1492
)
