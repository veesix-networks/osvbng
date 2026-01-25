package ppp

import (
	"testing"
)

func TestNewIPv6CP(t *testing.T) {
	ipv6cp := NewIPv6CP(Callbacks{})
	if ipv6cp.FSM().State() != Initial {
		t.Errorf("expected Initial state, got %v", ipv6cp.FSM().State())
	}

	id := ipv6cp.LocalConfig().InterfaceID
	allZero := true
	for _, b := range id {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("expected non-zero interface ID")
	}

	if id[0]&0x02 != 0 {
		t.Error("expected u-bit to be 0 for random ID")
	}
}

func TestIPv6CPConfigFromMAC(t *testing.T) {
	mac := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
	cfg := IPv6CPConfigFromMAC(mac)

	expected := [8]byte{0x02, 0x11, 0x22, 0xFF, 0xFE, 0x33, 0x44, 0x55}
	if cfg.InterfaceID != expected {
		t.Errorf("expected %x, got %x", expected, cfg.InterfaceID)
	}
}

func TestIPv6CPConfigFromMACUBit(t *testing.T) {
	mac := []byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}
	cfg := IPv6CPConfigFromMAC(mac)

	if cfg.InterfaceID[0] != 0x00 {
		t.Errorf("expected u-bit flipped to 0x00, got %02x", cfg.InterfaceID[0])
	}
}

func TestIPv6CPSetInterfaceID(t *testing.T) {
	ipv6cp := NewIPv6CP(Callbacks{})

	id := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	ipv6cp.SetInterfaceID(id)

	if ipv6cp.LocalConfig().InterfaceID != id {
		t.Errorf("expected %x, got %x", id, ipv6cp.LocalConfig().InterfaceID)
	}
}

func TestIPv6CPBuildConfReq(t *testing.T) {
	ipv6cp := NewIPv6CP(Callbacks{})
	opts := ipv6cp.BuildConfReq()

	if len(opts) != 1 {
		t.Errorf("expected 1 option, got %d", len(opts))
	}
	if opts[0].Type != IPv6CPOptInterfaceID {
		t.Errorf("expected InterfaceID option, got %d", opts[0].Type)
	}
	if len(opts[0].Data) != 8 {
		t.Errorf("expected 8 bytes, got %d", len(opts[0].Data))
	}
}

func TestIPv6CPProcessConfReqDifferentID(t *testing.T) {
	ipv6cp := NewIPv6CP(Callbacks{})
	ipv6cp.SetInterfaceID([8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08})

	peerID := [8]byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88}
	opt := InterfaceIDOption(peerID)

	ack, nak, rej := ipv6cp.ProcessConfReq([]Option{opt})

	if len(ack) != 1 {
		t.Errorf("expected 1 ACK, got %d", len(ack))
	}
	if len(nak) != 0 {
		t.Errorf("expected 0 NAK, got %d", len(nak))
	}
	if len(rej) != 0 {
		t.Errorf("expected 0 REJ, got %d", len(rej))
	}
	if ipv6cp.PeerConfig().InterfaceID != peerID {
		t.Error("peer ID not set")
	}
}

func TestIPv6CPProcessConfReqZeroID(t *testing.T) {
	ipv6cp := NewIPv6CP(Callbacks{})
	ipv6cp.SetInterfaceID([8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08})

	zeroID := [8]byte{}
	opt := InterfaceIDOption(zeroID)

	_, nak, _ := ipv6cp.ProcessConfReq([]Option{opt})

	if len(nak) != 1 {
		t.Fatalf("expected 1 NAK, got %d", len(nak))
	}

	suggestedID := ParseInterfaceID(nak[0])
	if suggestedID == ipv6cp.LocalConfig().InterfaceID {
		t.Error("suggested ID should differ from local ID")
	}

	allZero := true
	for _, b := range suggestedID {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("suggested ID should be non-zero")
	}
}

func TestIPv6CPProcessConfReqSameID(t *testing.T) {
	ipv6cp := NewIPv6CP(Callbacks{})
	localID := ipv6cp.LocalConfig().InterfaceID

	opt := InterfaceIDOption(localID)
	_, nak, _ := ipv6cp.ProcessConfReq([]Option{opt})

	if len(nak) != 1 {
		t.Fatalf("expected 1 NAK for same ID, got %d", len(nak))
	}

	suggestedID := ParseInterfaceID(nak[0])
	if suggestedID == localID {
		t.Error("suggested ID should differ from local ID")
	}
}

func TestIPv6CPProcessConfReqBothZero(t *testing.T) {
	ipv6cp := NewIPv6CP(Callbacks{})
	ipv6cp.SetInterfaceID([8]byte{})

	zeroID := [8]byte{}
	opt := InterfaceIDOption(zeroID)

	_, _, rej := ipv6cp.ProcessConfReq([]Option{opt})

	if len(rej) != 1 {
		t.Errorf("expected REJ when both IDs are zero, got %d", len(rej))
	}
}

func TestIPv6CPProcessConfReqUnknownOption(t *testing.T) {
	ipv6cp := NewIPv6CP(Callbacks{})

	opt := Option{Type: 99, Data: []byte{0x01, 0x02}}
	_, _, rej := ipv6cp.ProcessConfReq([]Option{opt})

	if len(rej) != 1 {
		t.Errorf("expected REJ for unknown option, got %d", len(rej))
	}
}

func TestIPv6CPProcessConfReqInvalidLength(t *testing.T) {
	ipv6cp := NewIPv6CP(Callbacks{})

	opt := Option{Type: IPv6CPOptInterfaceID, Data: []byte{0x01, 0x02, 0x03}}
	_, _, rej := ipv6cp.ProcessConfReq([]Option{opt})

	if len(rej) != 1 {
		t.Errorf("expected REJ for invalid length, got %d", len(rej))
	}
}

func TestIPv6CPProcessConfAck(t *testing.T) {
	ipv6cp := NewIPv6CP(Callbacks{})

	id := [8]byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x00, 0x11}
	opts := []Option{InterfaceIDOption(id)}

	ipv6cp.ProcessConfAck(opts)

	if ipv6cp.LocalConfig().InterfaceID != id {
		t.Errorf("expected %x, got %x", id, ipv6cp.LocalConfig().InterfaceID)
	}
}

func TestIPv6CPProcessConfNak(t *testing.T) {
	ipv6cp := NewIPv6CP(Callbacks{})

	id := [8]byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88}
	opts := []Option{InterfaceIDOption(id)}

	ipv6cp.ProcessConfNak(opts)

	if ipv6cp.LocalConfig().InterfaceID != id {
		t.Errorf("expected %x, got %x", id, ipv6cp.LocalConfig().InterfaceID)
	}
}

func TestIPv6CPProcessConfRej(t *testing.T) {
	ipv6cp := NewIPv6CP(Callbacks{})

	opts := []Option{{Type: IPv6CPOptInterfaceID, Data: make([]byte, 8)}}
	ipv6cp.ProcessConfRej(opts)

	newOpts := ipv6cp.BuildConfReq()
	if len(newOpts) != 0 {
		t.Error("InterfaceID should be excluded after rejection")
	}
}

func TestInterfaceIDOption(t *testing.T) {
	id := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	opt := InterfaceIDOption(id)

	if opt.Type != IPv6CPOptInterfaceID {
		t.Errorf("expected type %d, got %d", IPv6CPOptInterfaceID, opt.Type)
	}
	if len(opt.Data) != 8 {
		t.Errorf("expected 8 bytes, got %d", len(opt.Data))
	}

	parsed := ParseInterfaceID(opt)
	if parsed != id {
		t.Errorf("ParseInterfaceID() = %x, want %x", parsed, id)
	}
}

func TestParseInterfaceIDWrongType(t *testing.T) {
	opt := Option{Type: 99, Data: make([]byte, 8)}
	parsed := ParseInterfaceID(opt)

	expected := [8]byte{}
	if parsed != expected {
		t.Error("expected zero for wrong type")
	}
}

func TestParseInterfaceIDWrongLength(t *testing.T) {
	opt := Option{Type: IPv6CPOptInterfaceID, Data: []byte{0x01, 0x02}}
	parsed := ParseInterfaceID(opt)

	expected := [8]byte{}
	if parsed != expected {
		t.Error("expected zero for wrong length")
	}
}

func TestMakeLinkLocalAddress(t *testing.T) {
	id := [8]byte{0x02, 0x11, 0x22, 0xFF, 0xFE, 0x33, 0x44, 0x55}
	addr := MakeLinkLocalAddress(id)

	if len(addr) != 16 {
		t.Fatalf("expected 16 bytes, got %d", len(addr))
	}
	if addr[0] != 0xFE || addr[1] != 0x80 {
		t.Errorf("expected FE80 prefix, got %02x%02x", addr[0], addr[1])
	}
	for i := 2; i < 8; i++ {
		if addr[i] != 0 {
			t.Errorf("expected zero at position %d, got %02x", i, addr[i])
		}
	}
	for i := 0; i < 8; i++ {
		if addr[8+i] != id[i] {
			t.Errorf("interface ID mismatch at position %d", i)
		}
	}
}

func TestParseIPv6CPPacket(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		wantCode uint8
		wantID   uint8
		wantErr  bool
	}{
		{
			name:     "valid packet",
			data:     []byte{1, 42, 0, 12, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
			wantCode: 1,
			wantID:   42,
			wantErr:  false,
		},
		{
			name:    "short header",
			data:    []byte{1, 42, 0},
			wantErr: true,
		},
		{
			name:    "length exceeds data",
			data:    []byte{1, 42, 0, 20, 0x01},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, id, _, err := ParseIPv6CPPacket(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseIPv6CPPacket() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if code != tt.wantCode {
					t.Errorf("code = %d, want %d", code, tt.wantCode)
				}
				if id != tt.wantID {
					t.Errorf("id = %d, want %d", id, tt.wantID)
				}
			}
		})
	}
}

func TestGeneratePeerInterfaceID(t *testing.T) {
	local := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}

	for i := 0; i < 10; i++ {
		peer := generatePeerInterfaceID(local)
		if peer == local {
			t.Error("peer ID should differ from local")
		}
		if peer[0]&0x02 != 0 {
			t.Error("u-bit should be 0")
		}
	}
}
