package ppp

import (
	"testing"
)

func TestFSMInitialState(t *testing.T) {
	fsm := NewFSM(ProtoLCP, Callbacks{}, nil)
	if fsm.State() != Initial {
		t.Errorf("expected Initial state, got %v", fsm.State())
	}
}

func TestFSMStateStrings(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{Initial, "Initial"},
		{Starting, "Starting"},
		{Closed, "Closed"},
		{Stopped, "Stopped"},
		{Closing, "Closing"},
		{Stopping, "Stopping"},
		{ReqSent, "Req-Sent"},
		{AckRcvd, "Ack-Rcvd"},
		{AckSent, "Ack-Sent"},
		{Opened, "Opened"},
		{State(99), "State(99)"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("State(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestFSMUp(t *testing.T) {
	fsm := NewFSM(ProtoLCP, Callbacks{}, &mockHandler{})

	fsm.Up()
	if fsm.State() != Closed {
		t.Errorf("Up from Initial: expected Closed, got %v", fsm.State())
	}
}

func TestFSMOpen(t *testing.T) {
	var sentCode uint8
	cb := Callbacks{
		Send: func(code uint8, id uint8, data []byte) {
			sentCode = code
		},
	}

	fsm := NewFSM(ProtoLCP, cb, &mockHandler{})

	fsm.Open()
	if fsm.State() != Starting {
		t.Errorf("Open from Initial: expected Starting, got %v", fsm.State())
	}

	fsm.Up()
	if fsm.State() != ReqSent {
		t.Errorf("Up from Starting: expected ReqSent, got %v", fsm.State())
	}
	if sentCode != ConfReq {
		t.Errorf("expected ConfReq to be sent, got %d", sentCode)
	}
}

func TestFSMOpenFromClosed(t *testing.T) {
	var sentCode uint8
	cb := Callbacks{
		Send: func(code uint8, id uint8, data []byte) {
			sentCode = code
		},
	}

	fsm := NewFSM(ProtoLCP, cb, &mockHandler{})
	fsm.Up()

	if fsm.State() != Closed {
		t.Fatalf("expected Closed, got %v", fsm.State())
	}

	fsm.Open()
	if fsm.State() != ReqSent {
		t.Errorf("Open from Closed: expected ReqSent, got %v", fsm.State())
	}
	if sentCode != ConfReq {
		t.Errorf("expected ConfReq to be sent, got %d", sentCode)
	}
}

func TestFSMClose(t *testing.T) {
	var layerFinished bool
	cb := Callbacks{
		Send:          func(code uint8, id uint8, data []byte) {},
		LayerFinished: func() { layerFinished = true },
	}

	fsm := NewFSM(ProtoLCP, cb, &mockHandler{})
	fsm.Open()

	fsm.Close()
	if fsm.State() != Initial {
		t.Errorf("Close from Starting: expected Initial, got %v", fsm.State())
	}
	if !layerFinished {
		t.Error("expected LayerFinished callback")
	}
}

func TestFSMDown(t *testing.T) {
	cb := Callbacks{
		Send: func(code uint8, id uint8, data []byte) {},
	}

	fsm := NewFSM(ProtoLCP, cb, &mockHandler{})
	fsm.Up()
	fsm.Open()

	fsm.Down()
	if fsm.State() != Starting {
		t.Errorf("Down from ReqSent: expected Starting, got %v", fsm.State())
	}
}

func TestFSMConfReqAck(t *testing.T) {
	var lastCode uint8
	var layerUp bool
	cb := Callbacks{
		Send:    func(code uint8, id uint8, data []byte) { lastCode = code },
		LayerUp: func() { layerUp = true },
	}

	handler := &mockHandler{ackAll: true}
	fsm := NewFSM(ProtoLCP, cb, handler)
	fsm.Up()
	fsm.Open()

	fsm.Input(ConfReq, 1, []byte{})
	if lastCode != ConfAck {
		t.Errorf("expected ConfAck, got %d", lastCode)
	}
	if fsm.State() != AckSent {
		t.Errorf("expected AckSent, got %v", fsm.State())
	}

	fsm.Input(ConfAck, fsm.id, []byte{})
	if fsm.State() != Opened {
		t.Errorf("expected Opened, got %v", fsm.State())
	}
	if !layerUp {
		t.Error("expected LayerUp callback")
	}
}

func TestFSMConfNak(t *testing.T) {
	var sendCount int
	cb := Callbacks{
		Send: func(code uint8, id uint8, data []byte) { sendCount++ },
	}

	handler := &mockHandler{ackAll: true}
	fsm := NewFSM(ProtoLCP, cb, handler)
	fsm.Up()
	fsm.Open()

	initialCount := sendCount
	fsm.Input(ConfNak, fsm.id, []byte{})

	if sendCount <= initialCount {
		t.Error("expected new ConfReq after ConfNak")
	}
}

func TestFSMConfRej(t *testing.T) {
	cb := Callbacks{
		Send: func(code uint8, id uint8, data []byte) {},
	}

	handler := &mockHandler{ackAll: true}
	fsm := NewFSM(ProtoLCP, cb, handler)
	fsm.Up()
	fsm.Open()

	fsm.Input(ConfRej, fsm.id, []byte{})
	if fsm.State() != ReqSent {
		t.Errorf("expected ReqSent after ConfRej, got %v", fsm.State())
	}
}

func TestFSMTermReq(t *testing.T) {
	var lastCode uint8
	var layerDown bool
	cb := Callbacks{
		Send:      func(code uint8, id uint8, data []byte) { lastCode = code },
		LayerUp:   func() {},
		LayerDown: func() { layerDown = true },
	}

	handler := &mockHandler{ackAll: true}
	fsm := NewFSM(ProtoLCP, cb, handler)
	fsm.Up()
	fsm.Open()
	fsm.Input(ConfReq, 1, []byte{})
	fsm.Input(ConfAck, fsm.id, []byte{})

	if fsm.State() != Opened {
		t.Fatalf("expected Opened, got %v", fsm.State())
	}

	fsm.Input(TermReq, 5, nil)
	if lastCode != TermAck {
		t.Errorf("expected TermAck, got %d", lastCode)
	}
	if !layerDown {
		t.Error("expected LayerDown callback")
	}
	if fsm.State() != Stopping {
		t.Errorf("expected Stopping, got %v", fsm.State())
	}
}

func TestFSMEchoReq(t *testing.T) {
	var lastCode uint8
	var lastData []byte
	cb := Callbacks{
		Send: func(code uint8, id uint8, data []byte) {
			lastCode = code
			lastData = data
		},
		LayerUp: func() {},
	}

	handler := &mockHandler{ackAll: true}
	fsm := NewFSM(ProtoLCP, cb, handler)
	fsm.Up()
	fsm.Open()
	fsm.Input(ConfReq, 1, []byte{})
	fsm.Input(ConfAck, fsm.id, []byte{})

	echoData := []byte{0x00, 0x01, 0x02, 0x03, 0xDE, 0xAD}
	fsm.Input(EchoReq, 10, echoData)

	if lastCode != EchoRep {
		t.Errorf("expected EchoRep, got %d", lastCode)
	}
	if len(lastData) != len(echoData) {
		t.Errorf("expected echo data length %d, got %d", len(echoData), len(lastData))
	}
}

func TestFSMCodeRej(t *testing.T) {
	var lastCode uint8
	cb := Callbacks{
		Send: func(code uint8, id uint8, data []byte) { lastCode = code },
	}

	handler := &mockHandler{ackAll: true}
	fsm := NewFSM(ProtoLCP, cb, handler)
	fsm.Up()
	fsm.Open()

	fsm.Input(200, 1, []byte{0x01, 0x02})
	if lastCode != CodeRej {
		t.Errorf("expected CodeRej for unknown code, got %d", lastCode)
	}
}

func TestParseOptions(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantLen int
		wantErr bool
	}{
		{"empty", []byte{}, 0, false},
		{"single option", []byte{1, 4, 0x05, 0xDC}, 1, false},
		{"two options", []byte{1, 4, 0x05, 0xDC, 5, 6, 0, 0, 0, 1}, 2, false},
		{"invalid length zero", []byte{1, 0}, 0, true},
		{"invalid length too long", []byte{1, 10, 0x01}, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := ParseOptions(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseOptions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(opts) != tt.wantLen {
				t.Errorf("ParseOptions() got %d options, want %d", len(opts), tt.wantLen)
			}
		})
	}
}

func TestSerializeOptions(t *testing.T) {
	opts := []Option{
		{Type: 1, Data: []byte{0x05, 0xDC}},
		{Type: 5, Data: []byte{0x00, 0x00, 0x00, 0x01}},
	}

	data := SerializeOptions(opts)
	expected := []byte{1, 4, 0x05, 0xDC, 5, 6, 0x00, 0x00, 0x00, 0x01}

	if len(data) != len(expected) {
		t.Fatalf("SerializeOptions() length = %d, want %d", len(data), len(expected))
	}
	for i := range data {
		if data[i] != expected[i] {
			t.Errorf("SerializeOptions()[%d] = %02x, want %02x", i, data[i], expected[i])
		}
	}
}

func TestOptionLen(t *testing.T) {
	o := Option{Type: 1, Data: []byte{0x05, 0xDC}}
	if o.Len() != 4 {
		t.Errorf("Option.Len() = %d, want 4", o.Len())
	}
}

type mockHandler struct {
	ackAll bool
}

func (m *mockHandler) BuildConfReq() []Option {
	return nil
}

func (m *mockHandler) ProcessConfReq(opts []Option) (ack, nak, rej []Option) {
	if m.ackAll {
		return opts, nil, nil
	}
	return nil, nil, opts
}

func (m *mockHandler) ProcessConfAck(opts []Option) {}
func (m *mockHandler) ProcessConfNak(opts []Option) {}
func (m *mockHandler) ProcessConfRej(opts []Option) {}
