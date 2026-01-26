package ppp

import (
	"testing"
)

func TestNewLCP(t *testing.T) {
	lcp := NewLCP(Callbacks{})
	if lcp.FSM().State() != Initial {
		t.Errorf("expected Initial state, got %v", lcp.FSM().State())
	}
	if lcp.LocalConfig().MRU != DefaultPPPoEMRU {
		t.Errorf("expected MRU %d, got %d", DefaultPPPoEMRU, lcp.LocalConfig().MRU)
	}
	if lcp.LocalConfig().Magic == 0 {
		t.Error("expected non-zero magic number")
	}
}

func TestLCPSetAuthProto(t *testing.T) {
	lcp := NewLCP(Callbacks{})
	lcp.SetAuthProto(ProtoCHAP, CHAPMD5)

	cfg := lcp.LocalConfig()
	if cfg.AuthProto != ProtoCHAP {
		t.Errorf("expected AuthProto %04x, got %04x", ProtoCHAP, cfg.AuthProto)
	}
	if cfg.AuthAlgo != CHAPMD5 {
		t.Errorf("expected AuthAlgo %d, got %d", CHAPMD5, cfg.AuthAlgo)
	}
	if !cfg.WantAuth {
		t.Error("expected WantAuth to be true")
	}
}

func TestLCPBuildConfReq(t *testing.T) {
	lcp := NewLCP(Callbacks{})
	opts := lcp.BuildConfReq()

	var hasMRU, hasMagic bool
	for _, o := range opts {
		switch o.Type {
		case LCPOptMRU:
			hasMRU = true
		case LCPOptMagic:
			hasMagic = true
		}
	}

	if !hasMRU {
		t.Error("expected MRU option in ConfReq")
	}
	if !hasMagic {
		t.Error("expected Magic option in ConfReq")
	}
}

func TestLCPBuildConfReqWithAuth(t *testing.T) {
	lcp := NewLCP(Callbacks{})
	lcp.SetAuthProto(ProtoPAP, 0)
	opts := lcp.BuildConfReq()

	var hasAuth bool
	for _, o := range opts {
		if o.Type == LCPOptAuthProto {
			hasAuth = true
		}
	}

	if !hasAuth {
		t.Error("expected Auth option in ConfReq")
	}
}

func TestLCPProcessConfReqMRU(t *testing.T) {
	lcp := NewLCP(Callbacks{})

	tests := []struct {
		name     string
		mru      uint16
		wantAck  bool
		wantNak  bool
	}{
		{"valid MRU", 1492, true, false},
		{"minimum MRU", 64, true, false},
		{"too small MRU", 32, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opt := MRUOption(tt.mru)
			ack, nak, rej := lcp.ProcessConfReq([]Option{opt})

			if tt.wantAck && len(ack) == 0 {
				t.Error("expected ACK")
			}
			if tt.wantNak && len(nak) == 0 {
				t.Error("expected NAK")
			}
			if len(rej) > 0 {
				t.Error("unexpected REJ")
			}
		})
	}
}

func TestLCPProcessConfReqMagic(t *testing.T) {
	lcp := NewLCP(Callbacks{})
	localMagic := lcp.LocalConfig().Magic

	tests := []struct {
		name    string
		magic   uint32
		wantAck bool
		wantNak bool
	}{
		{"different magic", 0xDEADBEEF, true, false},
		{"same magic", localMagic, false, true},
		{"zero magic", 0, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opt := MagicOption(tt.magic)
			ack, nak, _ := lcp.ProcessConfReq([]Option{opt})

			if tt.wantAck && len(ack) == 0 {
				t.Error("expected ACK")
			}
			if tt.wantNak && len(nak) == 0 {
				t.Error("expected NAK")
			}
		})
	}
}

func TestLCPProcessConfReqAuth(t *testing.T) {
	lcp := NewLCP(Callbacks{})

	tests := []struct {
		name    string
		proto   uint16
		algo    uint8
		wantAck bool
		wantNak bool
	}{
		{"PAP", ProtoPAP, 0, true, false},
		{"CHAP MD5", ProtoCHAP, CHAPMD5, true, false},
		{"unknown proto", 0x1234, 0, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opt := AuthOption(tt.proto, tt.algo)
			ack, nak, _ := lcp.ProcessConfReq([]Option{opt})

			if tt.wantAck && len(ack) == 0 {
				t.Error("expected ACK")
			}
			if tt.wantNak && len(nak) == 0 {
				t.Error("expected NAK")
			}
		})
	}
}

func TestLCPProcessConfReqPFCACFC(t *testing.T) {
	lcp := NewLCP(Callbacks{})

	opts := []Option{
		{Type: LCPOptPFC, Data: nil},
		{Type: LCPOptACFC, Data: nil},
	}

	_, _, rej := lcp.ProcessConfReq(opts)
	if len(rej) != 2 {
		t.Errorf("expected 2 rejected options, got %d", len(rej))
	}
}

func TestLCPProcessConfAck(t *testing.T) {
	lcp := NewLCP(Callbacks{})

	opts := []Option{
		MRUOption(1400),
		MagicOption(0x12345678),
	}

	lcp.ProcessConfAck(opts)

	if lcp.LocalConfig().MRU != 1400 {
		t.Errorf("expected MRU 1400, got %d", lcp.LocalConfig().MRU)
	}
	if lcp.LocalConfig().Magic != 0x12345678 {
		t.Errorf("expected Magic 0x12345678, got 0x%08x", lcp.LocalConfig().Magic)
	}
}

func TestLCPProcessConfNak(t *testing.T) {
	lcp := NewLCP(Callbacks{})

	opts := []Option{
		MRUOption(1400),
		MagicOption(0xABCDEF00),
		AuthOption(ProtoCHAP, CHAPMD5),
	}

	lcp.ProcessConfNak(opts)

	if lcp.LocalConfig().MRU != 1400 {
		t.Errorf("expected MRU 1400, got %d", lcp.LocalConfig().MRU)
	}
	if lcp.LocalConfig().Magic != 0xABCDEF00 {
		t.Errorf("expected Magic 0xABCDEF00, got 0x%08x", lcp.LocalConfig().Magic)
	}
	if lcp.LocalConfig().AuthProto != ProtoCHAP {
		t.Errorf("expected AuthProto CHAP, got %04x", lcp.LocalConfig().AuthProto)
	}
}

func TestLCPProcessConfRej(t *testing.T) {
	lcp := NewLCP(Callbacks{})

	opts := []Option{
		{Type: LCPOptMagic, Data: []byte{0, 0, 0, 0}},
	}

	lcp.ProcessConfRej(opts)

	newOpts := lcp.BuildConfReq()
	for _, o := range newOpts {
		if o.Type == LCPOptMagic {
			t.Error("Magic should be excluded after rejection")
		}
	}
}

func TestMRUOption(t *testing.T) {
	opt := MRUOption(1492)
	if opt.Type != LCPOptMRU {
		t.Errorf("expected type %d, got %d", LCPOptMRU, opt.Type)
	}
	if len(opt.Data) != 2 {
		t.Errorf("expected 2 bytes, got %d", len(opt.Data))
	}

	mru := ParseMRU(opt)
	if mru != 1492 {
		t.Errorf("ParseMRU() = %d, want 1492", mru)
	}
}

func TestMagicOption(t *testing.T) {
	opt := MagicOption(0xDEADBEEF)
	if opt.Type != LCPOptMagic {
		t.Errorf("expected type %d, got %d", LCPOptMagic, opt.Type)
	}
	if len(opt.Data) != 4 {
		t.Errorf("expected 4 bytes, got %d", len(opt.Data))
	}

	magic := ParseMagic(opt)
	if magic != 0xDEADBEEF {
		t.Errorf("ParseMagic() = 0x%08x, want 0xDEADBEEF", magic)
	}
}

func TestAuthOptionCHAP(t *testing.T) {
	opt := AuthOption(ProtoCHAP, CHAPMD5)
	if opt.Type != LCPOptAuthProto {
		t.Errorf("expected type %d, got %d", LCPOptAuthProto, opt.Type)
	}
	if len(opt.Data) != 3 {
		t.Errorf("expected 3 bytes for CHAP, got %d", len(opt.Data))
	}

	proto, algo := ParseAuth(opt)
	if proto != ProtoCHAP {
		t.Errorf("ParseAuth() proto = %04x, want %04x", proto, ProtoCHAP)
	}
	if algo != CHAPMD5 {
		t.Errorf("ParseAuth() algo = %d, want %d", algo, CHAPMD5)
	}
}

func TestAuthOptionPAP(t *testing.T) {
	opt := AuthOption(ProtoPAP, 0)
	if len(opt.Data) != 2 {
		t.Errorf("expected 2 bytes for PAP, got %d", len(opt.Data))
	}

	proto, algo := ParseAuth(opt)
	if proto != ProtoPAP {
		t.Errorf("ParseAuth() proto = %04x, want %04x", proto, ProtoPAP)
	}
	if algo != 0 {
		t.Errorf("ParseAuth() algo = %d, want 0", algo)
	}
}

func TestEchoHandler(t *testing.T) {
	var sentCode uint8
	var sentID uint8
	var sentData []byte

	h := &EchoHandler{
		Magic: 0x12345678,
		Send: func(code uint8, id uint8, data []byte) {
			sentCode = code
			sentID = id
			sentData = data
		},
	}

	h.SendEchoReq(42)
	if sentCode != EchoReq {
		t.Errorf("expected EchoReq, got %d", sentCode)
	}
	if sentID != 42 {
		t.Errorf("expected ID 42, got %d", sentID)
	}

	reqData := []byte{0xAB, 0xCD, 0xEF, 0x00, 0x11, 0x22}
	h.HandleEchoReq(99, reqData)
	if sentCode != EchoRep {
		t.Errorf("expected EchoRep, got %d", sentCode)
	}
	if sentID != 99 {
		t.Errorf("expected ID 99, got %d", sentID)
	}
	if sentData[0] != 0x12 || sentData[1] != 0x34 || sentData[2] != 0x56 || sentData[3] != 0x78 {
		t.Error("expected local magic in response")
	}
}

func TestParseMAC(t *testing.T) {
	mac := ParseMAC([]byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55})
	if mac == nil {
		t.Fatal("expected non-nil MAC")
	}
	if mac.String() != "00:11:22:33:44:55" {
		t.Errorf("got %s", mac.String())
	}

	nilMAC := ParseMAC([]byte{0x00, 0x11})
	if nilMAC != nil {
		t.Error("expected nil for short input")
	}
}
