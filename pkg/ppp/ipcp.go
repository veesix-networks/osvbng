package ppp

import (
	"encoding/binary"
	"net"
)

type IPCPConfig struct {
	Address      net.IP
	PrimaryDNS   net.IP
	SecondaryDNS net.IP
	PeerAddress  net.IP
}

func DefaultIPCPConfig() IPCPConfig {
	return IPCPConfig{
		Address:      net.IPv4zero,
		PrimaryDNS:   net.IPv4zero,
		SecondaryDNS: net.IPv4zero,
	}
}

type IPCP struct {
	fsm      *FSM
	local    IPCPConfig
	peer     IPCPConfig
	rejected map[uint8]bool
}

func NewIPCP(cb Callbacks) *IPCP {
	i := &IPCP{
		local:    DefaultIPCPConfig(),
		rejected: make(map[uint8]bool),
	}
	i.fsm = NewFSM(ProtoIPCP, cb, i)
	return i
}

func (i *IPCP) FSM() *FSM               { return i.fsm }
func (i *IPCP) LocalConfig() IPCPConfig { return i.local }
func (i *IPCP) PeerConfig() IPCPConfig  { return i.peer }

func (i *IPCP) SetAddress(addr net.IP) {
	i.local.Address = addr.To4()
}

func (i *IPCP) SetPeerAddress(addr net.IP) {
	i.peer.PeerAddress = addr.To4()
}

func (i *IPCP) SetDNS(primary, secondary net.IP) {
	i.local.PrimaryDNS = primary.To4()
	i.local.SecondaryDNS = secondary.To4()
}

func (i *IPCP) BuildConfReq() []Option {
	var opts []Option

	if !i.rejected[IPCPOptAddress] {
		opts = append(opts, IPAddressOption(i.local.Address))
	}
	if !i.rejected[IPCPOptPrimaryDNS] {
		opts = append(opts, DNSOption(IPCPOptPrimaryDNS, i.local.PrimaryDNS))
	}
	if !i.rejected[IPCPOptSecondaryDNS] {
		opts = append(opts, DNSOption(IPCPOptSecondaryDNS, i.local.SecondaryDNS))
	}

	return opts
}

func (i *IPCP) ProcessConfReq(opts []Option) (ack, nak, rej []Option) {
	for _, o := range opts {
		switch o.Type {
		case IPCPOptAddress:
			if len(o.Data) == 4 {
				addr := net.IP(o.Data)
				if addr.Equal(net.IPv4zero) {
					if i.peer.PeerAddress != nil && !i.peer.PeerAddress.Equal(net.IPv4zero) {
						nak = append(nak, IPAddressOption(i.peer.PeerAddress))
					} else {
						rej = append(rej, o)
					}
				} else {
					i.peer.Address = addr
					ack = append(ack, o)
				}
			} else {
				rej = append(rej, o)
			}
		case IPCPOptPrimaryDNS:
			if len(o.Data) == 4 {
				addr := net.IP(o.Data)
				if addr.Equal(net.IPv4zero) && i.local.PrimaryDNS != nil && !i.local.PrimaryDNS.Equal(net.IPv4zero) {
					nak = append(nak, DNSOption(IPCPOptPrimaryDNS, i.local.PrimaryDNS))
				} else {
					i.peer.PrimaryDNS = addr
					ack = append(ack, o)
				}
			} else {
				rej = append(rej, o)
			}
		case IPCPOptSecondaryDNS:
			if len(o.Data) == 4 {
				addr := net.IP(o.Data)
				if addr.Equal(net.IPv4zero) && i.local.SecondaryDNS != nil && !i.local.SecondaryDNS.Equal(net.IPv4zero) {
					nak = append(nak, DNSOption(IPCPOptSecondaryDNS, i.local.SecondaryDNS))
				} else {
					i.peer.SecondaryDNS = addr
					ack = append(ack, o)
				}
			} else {
				rej = append(rej, o)
			}
		case IPCPOptCompression:
			rej = append(rej, o)
		case IPCPOptAddresses:
			rej = append(rej, o)
		case IPCPOptPrimaryNBNS, IPCPOptSecondaryNBNS:
			rej = append(rej, o)
		default:
			rej = append(rej, o)
		}
	}
	return
}

func (i *IPCP) ProcessConfAck(opts []Option) {
	for _, o := range opts {
		switch o.Type {
		case IPCPOptAddress:
			if len(o.Data) == 4 {
				i.local.Address = net.IP(o.Data)
			}
		case IPCPOptPrimaryDNS:
			if len(o.Data) == 4 {
				i.local.PrimaryDNS = net.IP(o.Data)
			}
		case IPCPOptSecondaryDNS:
			if len(o.Data) == 4 {
				i.local.SecondaryDNS = net.IP(o.Data)
			}
		}
	}
}

func (i *IPCP) ProcessConfNak(opts []Option) {
	for _, o := range opts {
		switch o.Type {
		case IPCPOptAddress:
			if len(o.Data) == 4 {
				i.local.Address = net.IP(o.Data)
			}
		case IPCPOptPrimaryDNS:
			if len(o.Data) == 4 {
				i.local.PrimaryDNS = net.IP(o.Data)
			}
		case IPCPOptSecondaryDNS:
			if len(o.Data) == 4 {
				i.local.SecondaryDNS = net.IP(o.Data)
			}
		}
	}
}

func (i *IPCP) ProcessConfRej(opts []Option) {
	for _, o := range opts {
		i.rejected[o.Type] = true
	}
}

func IPAddressOption(addr net.IP) Option {
	ip := addr.To4()
	if ip == nil {
		ip = make([]byte, 4)
	}
	return Option{Type: IPCPOptAddress, Data: []byte(ip)}
}

func DNSOption(optType uint8, addr net.IP) Option {
	ip := addr.To4()
	if ip == nil {
		ip = make([]byte, 4)
	}
	return Option{Type: optType, Data: []byte(ip)}
}

func ParseIPAddress(o Option) net.IP {
	if o.Type == IPCPOptAddress && len(o.Data) == 4 {
		return net.IP(o.Data)
	}
	return nil
}

func ParseDNS(o Option) net.IP {
	if (o.Type == IPCPOptPrimaryDNS || o.Type == IPCPOptSecondaryDNS) && len(o.Data) == 4 {
		return net.IP(o.Data)
	}
	return nil
}

func ParseCompression(o Option) (uint16, []byte) {
	if o.Type == IPCPOptCompression && len(o.Data) >= 2 {
		proto := binary.BigEndian.Uint16(o.Data[:2])
		return proto, o.Data[2:]
	}
	return 0, nil
}
