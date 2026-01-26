package ethernet

const (
	EtherTypeIPv4          uint16 = 0x0800
	EtherTypeARP           uint16 = 0x0806
	EtherTypeVLAN          uint16 = 0x8100
	EtherTypeQinQ          uint16 = 0x88A8
	EtherTypeIPv6          uint16 = 0x86DD
	EtherTypePPPoEDiscovery uint16 = 0x8863
	EtherTypePPPoESession   uint16 = 0x8864
)
