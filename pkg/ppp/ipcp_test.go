package ppp

import (
	"net"
	"testing"
)

func TestNewIPCP(t *testing.T) {
	ipcp := NewIPCP(Callbacks{})
	if ipcp.FSM().State() != Initial {
		t.Errorf("expected Initial state, got %v", ipcp.FSM().State())
	}
}

func TestIPCPSetAddress(t *testing.T) {
	ipcp := NewIPCP(Callbacks{})
	ipcp.SetAddress(net.ParseIP("10.0.0.1"))

	if !ipcp.LocalConfig().Address.Equal(net.ParseIP("10.0.0.1")) {
		t.Errorf("expected 10.0.0.1, got %v", ipcp.LocalConfig().Address)
	}
}

func TestIPCPSetDNS(t *testing.T) {
	ipcp := NewIPCP(Callbacks{})
	ipcp.SetDNS(net.ParseIP("8.8.8.8"), net.ParseIP("8.8.4.4"))

	if !ipcp.LocalConfig().PrimaryDNS.Equal(net.ParseIP("8.8.8.8")) {
		t.Errorf("expected primary 8.8.8.8, got %v", ipcp.LocalConfig().PrimaryDNS)
	}
	if !ipcp.LocalConfig().SecondaryDNS.Equal(net.ParseIP("8.8.4.4")) {
		t.Errorf("expected secondary 8.8.4.4, got %v", ipcp.LocalConfig().SecondaryDNS)
	}
}

func TestIPCPBuildConfReq(t *testing.T) {
	ipcp := NewIPCP(Callbacks{})
	opts := ipcp.BuildConfReq()

	var hasAddr, hasPrimaryDNS, hasSecondaryDNS bool
	for _, o := range opts {
		switch o.Type {
		case IPCPOptAddress:
			hasAddr = true
		case IPCPOptPrimaryDNS:
			hasPrimaryDNS = true
		case IPCPOptSecondaryDNS:
			hasSecondaryDNS = true
		}
	}

	if !hasAddr {
		t.Error("expected Address option")
	}
	if !hasPrimaryDNS {
		t.Error("expected Primary DNS option")
	}
	if !hasSecondaryDNS {
		t.Error("expected Secondary DNS option")
	}
}

func TestIPCPProcessConfReqAddress(t *testing.T) {
	ipcp := NewIPCP(Callbacks{})

	opt := IPAddressOption(net.ParseIP("192.168.1.100"))
	ack, _, rej := ipcp.ProcessConfReq([]Option{opt})

	if len(ack) != 1 {
		t.Errorf("expected 1 ACK, got %d", len(ack))
	}
	if len(rej) != 0 {
		t.Errorf("expected 0 REJ, got %d", len(rej))
	}
	if !ipcp.PeerConfig().Address.Equal(net.ParseIP("192.168.1.100")) {
		t.Errorf("peer address not set correctly")
	}
}

func TestIPCPProcessConfReqZeroAddress(t *testing.T) {
	ipcp := NewIPCP(Callbacks{})

	opt := IPAddressOption(net.IPv4zero)
	_, _, rej := ipcp.ProcessConfReq([]Option{opt})

	if len(rej) != 1 {
		t.Errorf("expected REJ for zero address without peer address set, got %d", len(rej))
	}
}

func TestIPCPProcessConfReqDNS(t *testing.T) {
	ipcp := NewIPCP(Callbacks{})
	ipcp.SetDNS(net.ParseIP("8.8.8.8"), net.ParseIP("8.8.4.4"))

	opts := []Option{
		DNSOption(IPCPOptPrimaryDNS, net.IPv4zero),
		DNSOption(IPCPOptSecondaryDNS, net.IPv4zero),
	}

	_, nak, _ := ipcp.ProcessConfReq(opts)

	if len(nak) != 2 {
		t.Errorf("expected 2 NAK for zero DNS, got %d", len(nak))
	}
}

func TestIPCPProcessConfReqCompression(t *testing.T) {
	ipcp := NewIPCP(Callbacks{})

	opt := Option{Type: IPCPOptCompression, Data: []byte{0x00, 0x2D}}
	_, _, rej := ipcp.ProcessConfReq([]Option{opt})

	if len(rej) != 1 {
		t.Errorf("expected REJ for compression, got %d", len(rej))
	}
}

func TestIPCPProcessConfReqNBNS(t *testing.T) {
	ipcp := NewIPCP(Callbacks{})

	opts := []Option{
		{Type: IPCPOptPrimaryNBNS, Data: []byte{0, 0, 0, 0}},
		{Type: IPCPOptSecondaryNBNS, Data: []byte{0, 0, 0, 0}},
	}

	_, _, rej := ipcp.ProcessConfReq(opts)

	if len(rej) != 2 {
		t.Errorf("expected 2 REJ for NBNS, got %d", len(rej))
	}
}

func TestIPCPProcessConfAck(t *testing.T) {
	ipcp := NewIPCP(Callbacks{})

	opts := []Option{
		IPAddressOption(net.ParseIP("10.0.0.50")),
		DNSOption(IPCPOptPrimaryDNS, net.ParseIP("8.8.8.8")),
		DNSOption(IPCPOptSecondaryDNS, net.ParseIP("8.8.4.4")),
	}

	ipcp.ProcessConfAck(opts)

	if !ipcp.LocalConfig().Address.Equal(net.ParseIP("10.0.0.50")) {
		t.Errorf("expected address 10.0.0.50, got %v", ipcp.LocalConfig().Address)
	}
	if !ipcp.LocalConfig().PrimaryDNS.Equal(net.ParseIP("8.8.8.8")) {
		t.Errorf("expected primary DNS 8.8.8.8, got %v", ipcp.LocalConfig().PrimaryDNS)
	}
	if !ipcp.LocalConfig().SecondaryDNS.Equal(net.ParseIP("8.8.4.4")) {
		t.Errorf("expected secondary DNS 8.8.4.4, got %v", ipcp.LocalConfig().SecondaryDNS)
	}
}

func TestIPCPProcessConfNak(t *testing.T) {
	ipcp := NewIPCP(Callbacks{})

	opts := []Option{
		IPAddressOption(net.ParseIP("172.16.0.1")),
		DNSOption(IPCPOptPrimaryDNS, net.ParseIP("1.1.1.1")),
	}

	ipcp.ProcessConfNak(opts)

	if !ipcp.LocalConfig().Address.Equal(net.ParseIP("172.16.0.1")) {
		t.Errorf("expected address 172.16.0.1, got %v", ipcp.LocalConfig().Address)
	}
	if !ipcp.LocalConfig().PrimaryDNS.Equal(net.ParseIP("1.1.1.1")) {
		t.Errorf("expected DNS 1.1.1.1, got %v", ipcp.LocalConfig().PrimaryDNS)
	}
}

func TestIPCPProcessConfRej(t *testing.T) {
	ipcp := NewIPCP(Callbacks{})

	opts := []Option{
		{Type: IPCPOptPrimaryDNS, Data: []byte{0, 0, 0, 0}},
	}

	ipcp.ProcessConfRej(opts)

	newOpts := ipcp.BuildConfReq()
	for _, o := range newOpts {
		if o.Type == IPCPOptPrimaryDNS {
			t.Error("Primary DNS should be excluded after rejection")
		}
	}
}

func TestIPAddressOption(t *testing.T) {
	opt := IPAddressOption(net.ParseIP("192.168.1.1"))
	if opt.Type != IPCPOptAddress {
		t.Errorf("expected type %d, got %d", IPCPOptAddress, opt.Type)
	}
	if len(opt.Data) != 4 {
		t.Errorf("expected 4 bytes, got %d", len(opt.Data))
	}

	addr := ParseIPAddress(opt)
	if !addr.Equal(net.ParseIP("192.168.1.1")) {
		t.Errorf("ParseIPAddress() = %v, want 192.168.1.1", addr)
	}
}

func TestIPAddressOptionNil(t *testing.T) {
	opt := IPAddressOption(nil)
	if len(opt.Data) != 4 {
		t.Errorf("expected 4 bytes for nil, got %d", len(opt.Data))
	}

	addr := ParseIPAddress(opt)
	if addr == nil {
		t.Error("expected non-nil address")
	}
	for _, b := range addr {
		if b != 0 {
			t.Errorf("expected all zeros, got %v", addr)
			break
		}
	}
}

func TestDNSOption(t *testing.T) {
	opt := DNSOption(IPCPOptPrimaryDNS, net.ParseIP("8.8.8.8"))
	if opt.Type != IPCPOptPrimaryDNS {
		t.Errorf("expected type %d, got %d", IPCPOptPrimaryDNS, opt.Type)
	}

	addr := ParseDNS(opt)
	if !addr.Equal(net.ParseIP("8.8.8.8")) {
		t.Errorf("ParseDNS() = %v, want 8.8.8.8", addr)
	}
}

func TestParseDNSWrongType(t *testing.T) {
	opt := Option{Type: IPCPOptAddress, Data: []byte{8, 8, 8, 8}}
	addr := ParseDNS(opt)
	if addr != nil {
		t.Error("expected nil for wrong type")
	}
}

func TestParseCompression(t *testing.T) {
	opt := Option{Type: IPCPOptCompression, Data: []byte{0x00, 0x2D, 0x0F, 0x01}}
	proto, extra := ParseCompression(opt)

	if proto != 0x002D {
		t.Errorf("expected proto 0x002D, got %04x", proto)
	}
	if len(extra) != 2 {
		t.Errorf("expected 2 extra bytes, got %d", len(extra))
	}
}

func TestParseCompressionShort(t *testing.T) {
	opt := Option{Type: IPCPOptCompression, Data: []byte{0x00}}
	proto, _ := ParseCompression(opt)
	if proto != 0 {
		t.Errorf("expected 0 for short data, got %04x", proto)
	}
}
