package ppp

import (
	"encoding/binary"
	"math/rand"
	"net"
)

type LCPConfig struct {
	MRU         uint16
	Magic       uint32
	AuthProto   uint16
	AuthAlgo    uint8
	WantAuth    bool
}

func DefaultLCPConfig() LCPConfig {
	return LCPConfig{
		MRU:   DefaultPPPoEMRU,
		Magic: rand.Uint32(),
	}
}

type LCP struct {
	fsm       *FSM
	local     LCPConfig
	peer      LCPConfig
	rejected  map[uint8]bool
}

func NewLCP(cb Callbacks) *LCP {
	l := &LCP{
		local:    DefaultLCPConfig(),
		rejected: make(map[uint8]bool),
	}
	l.fsm = NewFSM(ProtoLCP, cb, l)
	return l
}

func (l *LCP) FSM() *FSM             { return l.fsm }
func (l *LCP) LocalConfig() LCPConfig { return l.local }
func (l *LCP) PeerConfig() LCPConfig  { return l.peer }

func (l *LCP) SetAuthProto(proto uint16, algo uint8) {
	l.local.AuthProto = proto
	l.local.AuthAlgo = algo
	l.local.WantAuth = true
}

func (l *LCP) BuildConfReq() []Option {
	var opts []Option

	if !l.rejected[LCPOptMRU] {
		opts = append(opts, MRUOption(l.local.MRU))
	}
	if !l.rejected[LCPOptMagic] && l.local.Magic != 0 {
		opts = append(opts, MagicOption(l.local.Magic))
	}
	if !l.rejected[LCPOptAuthProto] && l.local.WantAuth {
		opts = append(opts, AuthOption(l.local.AuthProto, l.local.AuthAlgo))
	}

	return opts
}

func (l *LCP) ProcessConfReq(opts []Option) (ack, nak, rej []Option) {
	for _, o := range opts {
		switch o.Type {
		case LCPOptMRU:
			if len(o.Data) == 2 {
				mru := binary.BigEndian.Uint16(o.Data)
				if mru < 64 {
					nak = append(nak, MRUOption(DefaultPPPoEMRU))
				} else {
					l.peer.MRU = mru
					ack = append(ack, o)
				}
			} else {
				rej = append(rej, o)
			}
		case LCPOptMagic:
			if len(o.Data) == 4 {
				magic := binary.BigEndian.Uint32(o.Data)
				if magic == l.local.Magic && magic != 0 {
					nak = append(nak, o)
				} else {
					l.peer.Magic = magic
					ack = append(ack, o)
				}
			} else {
				rej = append(rej, o)
			}
		case LCPOptAuthProto:
			if len(o.Data) >= 2 {
				proto := binary.BigEndian.Uint16(o.Data)
				if proto == ProtoPAP || proto == ProtoCHAP {
					l.peer.AuthProto = proto
					if len(o.Data) > 2 {
						l.peer.AuthAlgo = o.Data[2]
					}
					ack = append(ack, o)
				} else {
					nak = append(nak, AuthOption(ProtoCHAP, CHAPMD5))
				}
			} else {
				rej = append(rej, o)
			}
		case LCPOptPFC, LCPOptACFC:
			rej = append(rej, o)
		default:
			rej = append(rej, o)
		}
	}
	return
}

func (l *LCP) ProcessConfAck(opts []Option) {
	for _, o := range opts {
		switch o.Type {
		case LCPOptMRU:
			if len(o.Data) == 2 {
				l.local.MRU = binary.BigEndian.Uint16(o.Data)
			}
		case LCPOptMagic:
			if len(o.Data) == 4 {
				l.local.Magic = binary.BigEndian.Uint32(o.Data)
			}
		}
	}
}

func (l *LCP) ProcessConfNak(opts []Option) {
	for _, o := range opts {
		switch o.Type {
		case LCPOptMRU:
			if len(o.Data) == 2 {
				l.local.MRU = binary.BigEndian.Uint16(o.Data)
			}
		case LCPOptMagic:
			if len(o.Data) == 4 {
				l.local.Magic = binary.BigEndian.Uint32(o.Data)
			}
		case LCPOptAuthProto:
			if len(o.Data) >= 2 {
				l.local.AuthProto = binary.BigEndian.Uint16(o.Data)
				if len(o.Data) > 2 {
					l.local.AuthAlgo = o.Data[2]
				}
			}
		}
	}
}

func (l *LCP) ProcessConfRej(opts []Option) {
	for _, o := range opts {
		l.rejected[o.Type] = true
	}
}

func MRUOption(mru uint16) Option {
	data := make([]byte, 2)
	binary.BigEndian.PutUint16(data, mru)
	return Option{Type: LCPOptMRU, Data: data}
}

func MagicOption(magic uint32) Option {
	data := make([]byte, 4)
	binary.BigEndian.PutUint32(data, magic)
	return Option{Type: LCPOptMagic, Data: data}
}

func AuthOption(proto uint16, algo uint8) Option {
	if proto == ProtoCHAP {
		data := make([]byte, 3)
		binary.BigEndian.PutUint16(data, proto)
		data[2] = algo
		return Option{Type: LCPOptAuthProto, Data: data}
	}
	data := make([]byte, 2)
	binary.BigEndian.PutUint16(data, proto)
	return Option{Type: LCPOptAuthProto, Data: data}
}

func ParseMRU(o Option) uint16 {
	if o.Type == LCPOptMRU && len(o.Data) == 2 {
		return binary.BigEndian.Uint16(o.Data)
	}
	return 0
}

func ParseMagic(o Option) uint32 {
	if o.Type == LCPOptMagic && len(o.Data) == 4 {
		return binary.BigEndian.Uint32(o.Data)
	}
	return 0
}

func ParseAuth(o Option) (uint16, uint8) {
	if o.Type == LCPOptAuthProto && len(o.Data) >= 2 {
		proto := binary.BigEndian.Uint16(o.Data)
		var algo uint8
		if len(o.Data) > 2 {
			algo = o.Data[2]
		}
		return proto, algo
	}
	return 0, 0
}

type EchoHandler struct {
	Magic uint32
	Send  func(code uint8, id uint8, data []byte)
}

func (h *EchoHandler) SendEchoReq(id uint8) {
	data := make([]byte, 4)
	binary.BigEndian.PutUint32(data, h.Magic)
	h.Send(EchoReq, id, data)
}

func (h *EchoHandler) HandleEchoReq(id uint8, data []byte) {
	if len(data) >= 4 {
		resp := make([]byte, len(data))
		binary.BigEndian.PutUint32(resp, h.Magic)
		copy(resp[4:], data[4:])
		h.Send(EchoRep, id, resp)
	}
}

func ParseMAC(b []byte) net.HardwareAddr {
	if len(b) == 6 {
		return net.HardwareAddr(b)
	}
	return nil
}
