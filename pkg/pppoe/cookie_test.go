package pppoe

import (
	"net"
	"testing"
	"time"
)

func TestCookieManager_GenerateValidate(t *testing.T) {
	mgr, err := NewCookieManager(60 * time.Second)
	if err != nil {
		t.Fatalf("failed to create cookie manager: %v", err)
	}

	mac, _ := net.ParseMAC("00:11:22:33:44:55")
	svlan := uint16(100)
	cvlan := uint16(200)

	cookie := mgr.Generate(mac, svlan, cvlan)
	if len(cookie) == 0 {
		t.Fatal("expected non-empty cookie")
	}

	if !mgr.Validate(cookie, mac, svlan, cvlan) {
		t.Error("cookie should be valid")
	}
}

func TestCookieManager_InvalidMAC(t *testing.T) {
	mgr, err := NewCookieManager(60 * time.Second)
	if err != nil {
		t.Fatalf("failed to create cookie manager: %v", err)
	}

	mac1, _ := net.ParseMAC("00:11:22:33:44:55")
	mac2, _ := net.ParseMAC("00:11:22:33:44:66")
	svlan := uint16(100)
	cvlan := uint16(200)

	cookie := mgr.Generate(mac1, svlan, cvlan)

	if mgr.Validate(cookie, mac2, svlan, cvlan) {
		t.Error("cookie should be invalid with different MAC")
	}
}

func TestCookieManager_InvalidSVLAN(t *testing.T) {
	mgr, err := NewCookieManager(60 * time.Second)
	if err != nil {
		t.Fatalf("failed to create cookie manager: %v", err)
	}

	mac, _ := net.ParseMAC("00:11:22:33:44:55")
	cvlan := uint16(200)

	cookie := mgr.Generate(mac, 100, cvlan)

	if mgr.Validate(cookie, mac, 101, cvlan) {
		t.Error("cookie should be invalid with different SVLAN")
	}
}

func TestCookieManager_InvalidCVLAN(t *testing.T) {
	mgr, err := NewCookieManager(60 * time.Second)
	if err != nil {
		t.Fatalf("failed to create cookie manager: %v", err)
	}

	mac, _ := net.ParseMAC("00:11:22:33:44:55")
	svlan := uint16(100)

	cookie := mgr.Generate(mac, svlan, 200)

	if mgr.Validate(cookie, mac, svlan, 201) {
		t.Error("cookie should be invalid with different CVLAN")
	}
}

func TestCookieManager_Expired(t *testing.T) {
	mgr, err := NewCookieManager(1 * time.Millisecond)
	if err != nil {
		t.Fatalf("failed to create cookie manager: %v", err)
	}

	mac, _ := net.ParseMAC("00:11:22:33:44:55")
	svlan := uint16(100)
	cvlan := uint16(200)

	cookie := mgr.Generate(mac, svlan, cvlan)

	// Wait for expiry
	time.Sleep(10 * time.Millisecond)

	if mgr.Validate(cookie, mac, svlan, cvlan) {
		t.Error("cookie should be expired")
	}
}

func TestCookieManager_TruncatedCookie(t *testing.T) {
	mgr, err := NewCookieManager(60 * time.Second)
	if err != nil {
		t.Fatalf("failed to create cookie manager: %v", err)
	}

	mac, _ := net.ParseMAC("00:11:22:33:44:55")

	// Cookie too short (less than 8 bytes for timestamp)
	shortCookie := []byte{0x01, 0x02, 0x03}
	if mgr.Validate(shortCookie, mac, 100, 200) {
		t.Error("truncated cookie should be invalid")
	}
}

func TestCookieManager_EmptyCookie(t *testing.T) {
	mgr, err := NewCookieManager(60 * time.Second)
	if err != nil {
		t.Fatalf("failed to create cookie manager: %v", err)
	}

	mac, _ := net.ParseMAC("00:11:22:33:44:55")

	if mgr.Validate(nil, mac, 100, 200) {
		t.Error("nil cookie should be invalid")
	}

	if mgr.Validate([]byte{}, mac, 100, 200) {
		t.Error("empty cookie should be invalid")
	}
}

func TestCookieManager_TamperedCookie(t *testing.T) {
	mgr, err := NewCookieManager(60 * time.Second)
	if err != nil {
		t.Fatalf("failed to create cookie manager: %v", err)
	}

	mac, _ := net.ParseMAC("00:11:22:33:44:55")
	svlan := uint16(100)
	cvlan := uint16(200)

	cookie := mgr.Generate(mac, svlan, cvlan)

	// Tamper with the HMAC portion
	if len(cookie) > 10 {
		cookie[10] ^= 0xFF
	}

	if mgr.Validate(cookie, mac, svlan, cvlan) {
		t.Error("tampered cookie should be invalid")
	}
}

func TestCookieManager_DifferentKeys(t *testing.T) {
	mgr1, err := NewCookieManager(60 * time.Second)
	if err != nil {
		t.Fatalf("failed to create cookie manager 1: %v", err)
	}

	mgr2, err := NewCookieManager(60 * time.Second)
	if err != nil {
		t.Fatalf("failed to create cookie manager 2: %v", err)
	}

	mac, _ := net.ParseMAC("00:11:22:33:44:55")
	svlan := uint16(100)
	cvlan := uint16(200)

	cookie := mgr1.Generate(mac, svlan, cvlan)

	// Different manager (different key) should reject
	if mgr2.Validate(cookie, mac, svlan, cvlan) {
		t.Error("cookie from different manager should be invalid")
	}
}

func TestCookieManager_ZeroVLANs(t *testing.T) {
	mgr, err := NewCookieManager(60 * time.Second)
	if err != nil {
		t.Fatalf("failed to create cookie manager: %v", err)
	}

	mac, _ := net.ParseMAC("00:11:22:33:44:55")

	// No VLANs
	cookie := mgr.Generate(mac, 0, 0)
	if !mgr.Validate(cookie, mac, 0, 0) {
		t.Error("cookie with zero VLANs should be valid")
	}

	// Should fail if VLANs added
	if mgr.Validate(cookie, mac, 100, 0) {
		t.Error("cookie should be invalid with different SVLAN")
	}
}

func TestCookieManager_CookieStructure(t *testing.T) {
	mgr, err := NewCookieManager(60 * time.Second)
	if err != nil {
		t.Fatalf("failed to create cookie manager: %v", err)
	}

	mac, _ := net.ParseMAC("00:11:22:33:44:55")
	cookie := mgr.Generate(mac, 100, 200)

	// Cookie should be 32 bytes HMAC-SHA256 + 4 bytes timestamp
	expectedLen := 32 + 4
	if len(cookie) != expectedLen {
		t.Errorf("expected cookie length %d, got %d", expectedLen, len(cookie))
	}
}

func TestCookieManager_ReplayWithinTTL(t *testing.T) {
	mgr, err := NewCookieManager(60 * time.Second)
	if err != nil {
		t.Fatalf("failed to create cookie manager: %v", err)
	}

	mac, _ := net.ParseMAC("00:11:22:33:44:55")
	svlan := uint16(100)
	cvlan := uint16(200)

	cookie := mgr.Generate(mac, svlan, cvlan)

	// Same cookie should validate multiple times within TTL
	for i := 0; i < 5; i++ {
		if !mgr.Validate(cookie, mac, svlan, cvlan) {
			t.Errorf("cookie should be valid on attempt %d", i+1)
		}
	}
}
