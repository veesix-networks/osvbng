package pppoe

import (
	"bytes"
	"testing"
)

func TestParseTags_Empty(t *testing.T) {
	tags, err := ParseTags([]byte{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tags.ServiceName != "" {
		t.Errorf("expected empty service name, got %q", tags.ServiceName)
	}
}

func TestParseTags_ServiceName(t *testing.T) {
	// Tag: 0x0101 (Service-Name), Length: 4, Value: "test"
	payload := []byte{0x01, 0x01, 0x00, 0x04, 't', 'e', 's', 't'}

	tags, err := ParseTags(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tags.ServiceName != "test" {
		t.Errorf("expected service name 'test', got %q", tags.ServiceName)
	}
}

func TestParseTags_EmptyServiceName(t *testing.T) {
	// Tag: 0x0101 (Service-Name), Length: 0
	payload := []byte{0x01, 0x01, 0x00, 0x00}

	tags, err := ParseTags(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tags.ServiceName != "" {
		t.Errorf("expected empty service name, got %q", tags.ServiceName)
	}
}

func TestParseTags_ACName(t *testing.T) {
	// Tag: 0x0102 (AC-Name), Length: 6, Value: "osvbng"
	payload := []byte{0x01, 0x02, 0x00, 0x06, 'o', 's', 'v', 'b', 'n', 'g'}

	tags, err := ParseTags(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tags.ACName != "osvbng" {
		t.Errorf("expected AC name 'osvbng', got %q", tags.ACName)
	}
}

func TestParseTags_HostUniq(t *testing.T) {
	// Tag: 0x0103 (Host-Uniq), Length: 4, Value: 0xDEADBEEF
	payload := []byte{0x01, 0x03, 0x00, 0x04, 0xDE, 0xAD, 0xBE, 0xEF}

	tags, err := ParseTags(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	if !bytes.Equal(tags.HostUniq, expected) {
		t.Errorf("expected Host-Uniq %x, got %x", expected, tags.HostUniq)
	}
}

func TestParseTags_ACCookie(t *testing.T) {
	// Tag: 0x0104 (AC-Cookie), Length: 4, Value: 0x12345678
	payload := []byte{0x01, 0x04, 0x00, 0x04, 0x12, 0x34, 0x56, 0x78}

	tags, err := ParseTags(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []byte{0x12, 0x34, 0x56, 0x78}
	if !bytes.Equal(tags.ACCookie, expected) {
		t.Errorf("expected AC-Cookie %x, got %x", expected, tags.ACCookie)
	}
}

func TestParseTags_RelaySessionID(t *testing.T) {
	// Tag: 0x0110 (Relay-Session-Id), Length: 2, Value: 0xABCD
	payload := []byte{0x01, 0x10, 0x00, 0x02, 0xAB, 0xCD}

	tags, err := ParseTags(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []byte{0xAB, 0xCD}
	if !bytes.Equal(tags.RelaySessionID, expected) {
		t.Errorf("expected Relay-Session-Id %x, got %x", expected, tags.RelaySessionID)
	}
}

func TestParseTags_MultipleTags(t *testing.T) {
	// Service-Name "isp" + Host-Uniq 0x1234
	payload := []byte{
		0x01, 0x01, 0x00, 0x03, 'i', 's', 'p', // Service-Name
		0x01, 0x03, 0x00, 0x02, 0x12, 0x34, // Host-Uniq
	}

	tags, err := ParseTags(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tags.ServiceName != "isp" {
		t.Errorf("expected service name 'isp', got %q", tags.ServiceName)
	}
	if !bytes.Equal(tags.HostUniq, []byte{0x12, 0x34}) {
		t.Errorf("expected Host-Uniq 0x1234, got %x", tags.HostUniq)
	}
}

func TestParseTags_EndOfList(t *testing.T) {
	// Service-Name + End-Of-List + more data (should stop at End-Of-List)
	payload := []byte{
		0x01, 0x01, 0x00, 0x03, 'f', 'o', 'o', // Service-Name
		0x00, 0x00, 0x00, 0x00, // End-Of-List
		0x01, 0x02, 0x00, 0x03, 'b', 'a', 'r', // AC-Name (should be ignored)
	}

	tags, err := ParseTags(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tags.ServiceName != "foo" {
		t.Errorf("expected service name 'foo', got %q", tags.ServiceName)
	}
	if tags.ACName != "" {
		t.Errorf("expected empty AC name after End-Of-List, got %q", tags.ACName)
	}
}

func TestParseTags_TruncatedHeader(t *testing.T) {
	// Only 3 bytes - not enough for tag header
	// Parser gracefully handles this by returning empty tags
	payload := []byte{0x01, 0x01, 0x00}

	tags, err := ParseTags(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No complete tags should be parsed
	if len(tags.Raw) != 0 {
		t.Errorf("expected no tags for truncated header, got %d", len(tags.Raw))
	}
}

func TestParseTags_TruncatedValue(t *testing.T) {
	// Header says length 10, but only 4 bytes of value
	payload := []byte{0x01, 0x01, 0x00, 0x0A, 't', 'e', 's', 't'}

	_, err := ParseTags(payload)
	if err == nil {
		t.Fatal("expected error for truncated value")
	}
}

func TestTagBuilder_ServiceName(t *testing.T) {
	data := NewTagBuilder().AddServiceName("test").Build()

	tags, err := ParseTags(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tags.ServiceName != "test" {
		t.Errorf("expected service name 'test', got %q", tags.ServiceName)
	}
}

func TestTagBuilder_EmptyServiceName(t *testing.T) {
	data := NewTagBuilder().AddServiceName("").Build()

	// Should produce: 0x0101, 0x0000 (tag type + zero length)
	if len(data) != 4 {
		t.Fatalf("expected 4 bytes, got %d", len(data))
	}

	tags, err := ParseTags(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tags.ServiceName != "" {
		t.Errorf("expected empty service name, got %q", tags.ServiceName)
	}
}

func TestTagBuilder_ACName(t *testing.T) {
	data := NewTagBuilder().AddACName("osvbng").Build()

	tags, err := ParseTags(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tags.ACName != "osvbng" {
		t.Errorf("expected AC name 'osvbng', got %q", tags.ACName)
	}
}

func TestTagBuilder_HostUniq(t *testing.T) {
	hostUniq := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	data := NewTagBuilder().AddHostUniq(hostUniq).Build()

	tags, err := ParseTags(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(tags.HostUniq, hostUniq) {
		t.Errorf("expected Host-Uniq %x, got %x", hostUniq, tags.HostUniq)
	}
}

func TestTagBuilder_ACCookie(t *testing.T) {
	cookie := []byte{0x12, 0x34, 0x56, 0x78}
	data := NewTagBuilder().AddACCookie(cookie).Build()

	tags, err := ParseTags(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(tags.ACCookie, cookie) {
		t.Errorf("expected AC-Cookie %x, got %x", cookie, tags.ACCookie)
	}
}

func TestTagBuilder_RelaySessionID(t *testing.T) {
	relayID := []byte{0xAB, 0xCD}
	data := NewTagBuilder().AddRelaySessionID(relayID).Build()

	tags, err := ParseTags(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(tags.RelaySessionID, relayID) {
		t.Errorf("expected Relay-Session-Id %x, got %x", relayID, tags.RelaySessionID)
	}
}

func TestTagBuilder_MultipleTags(t *testing.T) {
	hostUniq := []byte{0x11, 0x22}
	cookie := []byte{0x33, 0x44, 0x55, 0x66}

	data := NewTagBuilder().
		AddServiceName("broadband").
		AddACName("osvbng").
		AddHostUniq(hostUniq).
		AddACCookie(cookie).
		Build()

	tags, err := ParseTags(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tags.ServiceName != "broadband" {
		t.Errorf("expected service name 'broadband', got %q", tags.ServiceName)
	}
	if tags.ACName != "osvbng" {
		t.Errorf("expected AC name 'osvbng', got %q", tags.ACName)
	}
	if !bytes.Equal(tags.HostUniq, hostUniq) {
		t.Errorf("expected Host-Uniq %x, got %x", hostUniq, tags.HostUniq)
	}
	if !bytes.Equal(tags.ACCookie, cookie) {
		t.Errorf("expected AC-Cookie %x, got %x", cookie, tags.ACCookie)
	}
}

func TestTagBuilder_Chaining(t *testing.T) {
	// Verify chaining returns same builder
	b := NewTagBuilder()
	b2 := b.AddServiceName("test")
	if b != b2 {
		t.Error("AddServiceName should return same builder for chaining")
	}
}
