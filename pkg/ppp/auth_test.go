package ppp

import (
	"testing"
)

func TestPAPSendAuthReq(t *testing.T) {
	var sentCode uint8
	var sentID uint8
	var sentData []byte

	h := &PAPHandler{
		Send: func(code uint8, id uint8, data []byte) {
			sentCode = code
			sentID = id
			sentData = data
		},
	}

	h.SendAuthReq(1, "user", "pass")

	if sentCode != PAPAuthReq {
		t.Errorf("expected PAPAuthReq, got %d", sentCode)
	}
	if sentID != 1 {
		t.Errorf("expected ID 1, got %d", sentID)
	}
	if len(sentData) != 10 {
		t.Errorf("expected 10 bytes, got %d", len(sentData))
	}
	if sentData[0] != 4 {
		t.Errorf("expected username length 4, got %d", sentData[0])
	}
	if string(sentData[1:5]) != "user" {
		t.Errorf("expected username 'user', got %q", sentData[1:5])
	}
	if sentData[5] != 4 {
		t.Errorf("expected password length 4, got %d", sentData[5])
	}
	if string(sentData[6:10]) != "pass" {
		t.Errorf("expected password 'pass', got %q", sentData[6:10])
	}
}

func TestPAPHandleAuthReqSuccess(t *testing.T) {
	var sentCode uint8

	h := &PAPHandler{
		Send: func(code uint8, id uint8, data []byte) {
			sentCode = code
		},
		Validate: func(username, password string) bool {
			return username == "admin" && password == "secret"
		},
	}

	data := []byte{5, 'a', 'd', 'm', 'i', 'n', 6, 's', 'e', 'c', 'r', 'e', 't'}
	h.HandleAuthReq(1, data)

	if sentCode != PAPAuthAck {
		t.Errorf("expected PAPAuthAck, got %d", sentCode)
	}
}

func TestPAPHandleAuthReqFailure(t *testing.T) {
	var sentCode uint8

	h := &PAPHandler{
		Send: func(code uint8, id uint8, data []byte) {
			sentCode = code
		},
		Validate: func(username, password string) bool {
			return false
		},
	}

	data := []byte{4, 'u', 's', 'e', 'r', 4, 'p', 'a', 's', 's'}
	h.HandleAuthReq(1, data)

	if sentCode != PAPAuthNak {
		t.Errorf("expected PAPAuthNak, got %d", sentCode)
	}
}

func TestPAPHandleAuthReqMalformed(t *testing.T) {
	var sentCode uint8

	h := &PAPHandler{
		Send: func(code uint8, id uint8, data []byte) {
			sentCode = code
		},
	}

	h.HandleAuthReq(1, []byte{})
	if sentCode != PAPAuthNak {
		t.Errorf("expected PAPAuthNak for empty data, got %d", sentCode)
	}

	h.HandleAuthReq(1, []byte{10})
	if sentCode != PAPAuthNak {
		t.Errorf("expected PAPAuthNak for short username, got %d", sentCode)
	}

	h.HandleAuthReq(1, []byte{2, 'a', 'b', 10})
	if sentCode != PAPAuthNak {
		t.Errorf("expected PAPAuthNak for short password, got %d", sentCode)
	}
}

func TestPAPHandleAuthAck(t *testing.T) {
	var success bool
	var message string

	h := &PAPHandler{
		OnResult: func(s bool, msg string) {
			success = s
			message = msg
		},
	}

	data := []byte{7, 'w', 'e', 'l', 'c', 'o', 'm', 'e'}
	h.HandleAuthAck(1, data)

	if !success {
		t.Error("expected success=true")
	}
	if message != "welcome" {
		t.Errorf("expected message 'welcome', got %q", message)
	}
}

func TestPAPHandleAuthNak(t *testing.T) {
	var success bool
	var message string

	h := &PAPHandler{
		OnResult: func(s bool, msg string) {
			success = s
			message = msg
		},
	}

	data := []byte{6, 'd', 'e', 'n', 'i', 'e', 'd'}
	h.HandleAuthNak(1, data)

	if success {
		t.Error("expected success=false")
	}
	if message != "denied" {
		t.Errorf("expected message 'denied', got %q", message)
	}
}

func TestCHAPSendChallenge(t *testing.T) {
	var sentCode uint8
	var sentData []byte

	h := &CHAPHandler{
		Send: func(code uint8, id uint8, data []byte) {
			sentCode = code
			sentData = data
		},
	}

	challenge := []byte{0x01, 0x02, 0x03, 0x04}
	h.SendChallenge(1, challenge, "server")

	if sentCode != CHAPChallenge {
		t.Errorf("expected CHAPChallenge, got %d", sentCode)
	}
	if sentData[0] != 4 {
		t.Errorf("expected challenge length 4, got %d", sentData[0])
	}
	if string(sentData[5:]) != "server" {
		t.Errorf("expected name 'server', got %q", sentData[5:])
	}
}

func TestCHAPHandleChallenge(t *testing.T) {
	var sentCode uint8
	var sentData []byte

	h := &CHAPHandler{
		Send: func(code uint8, id uint8, data []byte) {
			sentCode = code
			sentData = data
		},
		LocalName:   "client",
		LocalSecret: "password",
	}

	challenge := []byte{4, 0x01, 0x02, 0x03, 0x04, 's', 'e', 'r', 'v', 'e', 'r'}
	h.HandleChallenge(5, challenge)

	if sentCode != CHAPResponse {
		t.Errorf("expected CHAPResponse, got %d", sentCode)
	}
	if sentData[0] != 16 {
		t.Errorf("expected MD5 response length 16, got %d", sentData[0])
	}
	if string(sentData[17:]) != "client" {
		t.Errorf("expected name 'client', got %q", sentData[17:])
	}
}

func TestCHAPHandleResponseSuccess(t *testing.T) {
	var sentCode uint8

	h := &CHAPHandler{
		Send: func(code uint8, id uint8, data []byte) {
			sentCode = code
		},
		GetSecret: func(username string) string {
			if username == "client" {
				return "password"
			}
			return ""
		},
	}

	challenge := []byte{0x01, 0x02, 0x03, 0x04}
	expected := h.computeMD5Response(5, challenge, "password")

	response := make([]byte, 1+16+6)
	response[0] = 16
	copy(response[1:17], expected)
	copy(response[17:], "client")

	h.HandleResponse(5, response, challenge)

	if sentCode != CHAPSuccess {
		t.Errorf("expected CHAPSuccess, got %d", sentCode)
	}
}

func TestCHAPHandleResponseFailure(t *testing.T) {
	var sentCode uint8

	h := &CHAPHandler{
		Send: func(code uint8, id uint8, data []byte) {
			sentCode = code
		},
		GetSecret: func(username string) string {
			return "wrong"
		},
	}

	challenge := []byte{0x01, 0x02, 0x03, 0x04}
	wrongResponse := make([]byte, 1+16+6)
	wrongResponse[0] = 16
	copy(wrongResponse[17:], "client")

	h.HandleResponse(5, wrongResponse, challenge)

	if sentCode != CHAPFailure {
		t.Errorf("expected CHAPFailure, got %d", sentCode)
	}
}

func TestCHAPHandleResponseMalformed(t *testing.T) {
	var sentCode uint8

	h := &CHAPHandler{
		Send: func(code uint8, id uint8, data []byte) {
			sentCode = code
		},
	}

	h.HandleResponse(5, []byte{}, []byte{0x01, 0x02})
	if sentCode != CHAPFailure {
		t.Errorf("expected CHAPFailure for empty response, got %d", sentCode)
	}

	h.HandleResponse(5, []byte{20}, []byte{0x01, 0x02})
	if sentCode != CHAPFailure {
		t.Errorf("expected CHAPFailure for short response, got %d", sentCode)
	}
}

func TestCHAPHandleSuccess(t *testing.T) {
	var success bool
	var message string

	h := &CHAPHandler{
		OnResult: func(s bool, msg string) {
			success = s
			message = msg
		},
	}

	h.HandleSuccess(1, []byte("welcome"))

	if !success {
		t.Error("expected success=true")
	}
	if message != "welcome" {
		t.Errorf("expected message 'welcome', got %q", message)
	}
}

func TestCHAPHandleFailure(t *testing.T) {
	var success bool
	var message string

	h := &CHAPHandler{
		OnResult: func(s bool, msg string) {
			success = s
			message = msg
		},
	}

	h.HandleFailure(1, []byte("denied"))

	if success {
		t.Error("expected success=false")
	}
	if message != "denied" {
		t.Errorf("expected message 'denied', got %q", message)
	}
}

func TestCHAPComputeMD5Response(t *testing.T) {
	h := &CHAPHandler{}

	response := h.computeMD5Response(1, []byte{0x01, 0x02, 0x03, 0x04}, "secret")

	if len(response) != 16 {
		t.Errorf("expected 16 bytes, got %d", len(response))
	}

	response2 := h.computeMD5Response(1, []byte{0x01, 0x02, 0x03, 0x04}, "secret")
	for i := range response {
		if response[i] != response2[i] {
			t.Error("expected deterministic result")
			break
		}
	}

	response3 := h.computeMD5Response(2, []byte{0x01, 0x02, 0x03, 0x04}, "secret")
	same := true
	for i := range response {
		if response[i] != response3[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("different ID should produce different response")
	}
}

func TestConstantTimeCompare(t *testing.T) {
	if !constantTimeCompare([]byte{1, 2, 3}, []byte{1, 2, 3}) {
		t.Error("equal slices should return true")
	}
	if constantTimeCompare([]byte{1, 2, 3}, []byte{1, 2, 4}) {
		t.Error("different slices should return false")
	}
	if constantTimeCompare([]byte{1, 2, 3}, []byte{1, 2}) {
		t.Error("different lengths should return false")
	}
}

func TestParsePAPPacket(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		wantCode uint8
		wantID   uint8
		wantErr  bool
	}{
		{
			name:     "valid packet",
			data:     []byte{1, 42, 0, 10, 4, 'u', 's', 'e', 'r', 0},
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
			code, id, _, err := ParsePAPPacket(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePAPPacket() error = %v, wantErr %v", err, tt.wantErr)
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

func TestParseCHAPPacket(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		wantCode uint8
		wantID   uint8
		wantErr  bool
	}{
		{
			name:     "valid packet",
			data:     []byte{1, 10, 0, 8, 4, 0x01, 0x02, 0x03},
			wantCode: 1,
			wantID:   10,
			wantErr:  false,
		},
		{
			name:    "short header",
			data:    []byte{1, 10, 0},
			wantErr: true,
		},
		{
			name:    "length exceeds data",
			data:    []byte{1, 10, 0, 20, 0x01},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, id, _, err := ParseCHAPPacket(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseCHAPPacket() error = %v, wantErr %v", err, tt.wantErr)
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

func TestErrString(t *testing.T) {
	if ErrShortPacket.Error() != "short packet" {
		t.Errorf("expected 'short packet', got %q", ErrShortPacket.Error())
	}
}
