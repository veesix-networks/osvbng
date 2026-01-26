package ppp

import (
	"crypto/rand"
	"encoding/binary"
)

type IPv6CPConfig struct {
	InterfaceID [8]byte
}

func DefaultIPv6CPConfig() IPv6CPConfig {
	var cfg IPv6CPConfig
	rand.Read(cfg.InterfaceID[:])
	cfg.InterfaceID[0] &= 0xFD
	return cfg
}

func IPv6CPConfigFromMAC(mac []byte) IPv6CPConfig {
	var cfg IPv6CPConfig
	if len(mac) == 6 {
		cfg.InterfaceID[0] = mac[0] ^ 0x02
		cfg.InterfaceID[1] = mac[1]
		cfg.InterfaceID[2] = mac[2]
		cfg.InterfaceID[3] = 0xFF
		cfg.InterfaceID[4] = 0xFE
		cfg.InterfaceID[5] = mac[3]
		cfg.InterfaceID[6] = mac[4]
		cfg.InterfaceID[7] = mac[5]
	}
	return cfg
}

type IPv6CP struct {
	fsm      *FSM
	local    IPv6CPConfig
	peer     IPv6CPConfig
	rejected map[uint8]bool
}

func NewIPv6CP(cb Callbacks) *IPv6CP {
	i := &IPv6CP{
		local:    DefaultIPv6CPConfig(),
		rejected: make(map[uint8]bool),
	}
	i.fsm = NewFSM(ProtoIPv6CP, cb, i)
	return i
}

func (i *IPv6CP) FSM() *FSM                  { return i.fsm }
func (i *IPv6CP) LocalConfig() IPv6CPConfig  { return i.local }
func (i *IPv6CP) PeerConfig() IPv6CPConfig   { return i.peer }

func (i *IPv6CP) SetInterfaceID(id [8]byte) {
	i.local.InterfaceID = id
}

func (i *IPv6CP) BuildConfReq() []Option {
	var opts []Option

	if !i.rejected[IPv6CPOptInterfaceID] {
		opts = append(opts, InterfaceIDOption(i.local.InterfaceID))
	}

	return opts
}

func (i *IPv6CP) ProcessConfReq(opts []Option) (ack, nak, rej []Option) {
	for _, o := range opts {
		switch o.Type {
		case IPv6CPOptInterfaceID:
			if len(o.Data) == 8 {
				var id [8]byte
				copy(id[:], o.Data)

				isZero := true
				for _, b := range id {
					if b != 0 {
						isZero = false
						break
					}
				}

				localZero := true
				for _, b := range i.local.InterfaceID {
					if b != 0 {
						localZero = false
						break
					}
				}

				if isZero && localZero {
					rej = append(rej, o)
				} else if isZero {
					suggested := generatePeerInterfaceID(i.local.InterfaceID)
					nak = append(nak, InterfaceIDOption(suggested))
				} else if id == i.local.InterfaceID {
					suggested := generatePeerInterfaceID(i.local.InterfaceID)
					nak = append(nak, InterfaceIDOption(suggested))
				} else {
					i.peer.InterfaceID = id
					ack = append(ack, o)
				}
			} else {
				rej = append(rej, o)
			}
		default:
			rej = append(rej, o)
		}
	}
	return
}

func (i *IPv6CP) ProcessConfAck(opts []Option) {
	for _, o := range opts {
		switch o.Type {
		case IPv6CPOptInterfaceID:
			if len(o.Data) == 8 {
				copy(i.local.InterfaceID[:], o.Data)
			}
		}
	}
}

func (i *IPv6CP) ProcessConfNak(opts []Option) {
	for _, o := range opts {
		switch o.Type {
		case IPv6CPOptInterfaceID:
			if len(o.Data) == 8 {
				copy(i.local.InterfaceID[:], o.Data)
			}
		}
	}
}

func (i *IPv6CP) ProcessConfRej(opts []Option) {
	for _, o := range opts {
		i.rejected[o.Type] = true
	}
}

func InterfaceIDOption(id [8]byte) Option {
	return Option{Type: IPv6CPOptInterfaceID, Data: id[:]}
}

func ParseInterfaceID(o Option) [8]byte {
	var id [8]byte
	if o.Type == IPv6CPOptInterfaceID && len(o.Data) == 8 {
		copy(id[:], o.Data)
	}
	return id
}

func generatePeerInterfaceID(local [8]byte) [8]byte {
	var peer [8]byte
	rand.Read(peer[:])
	peer[0] &= 0xFD

	for peer == local {
		rand.Read(peer[:])
		peer[0] &= 0xFD
	}
	return peer
}

func MakeLinkLocalAddress(interfaceID [8]byte) []byte {
	addr := make([]byte, 16)
	addr[0] = 0xFE
	addr[1] = 0x80
	copy(addr[8:], interfaceID[:])
	return addr
}

func ParseIPv6CPPacket(data []byte) (code uint8, id uint8, payload []byte, err error) {
	if len(data) < 4 {
		return 0, 0, nil, ErrShortPacket
	}
	code = data[0]
	id = data[1]
	length := binary.BigEndian.Uint16(data[2:4])
	if int(length) > len(data) {
		return 0, 0, nil, ErrShortPacket
	}
	return code, id, data[4:length], nil
}
