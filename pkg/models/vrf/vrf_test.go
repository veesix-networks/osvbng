package vrf

import "testing"

func TestValidateVRFName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"customers", false},
		{"vrf-1", false},
		{"VRF_test.1", false},
		{"a", false},
		{"1abc", false},
		{"", true},
		{"this-name-is-way-too-long", true},
		{"-invalid", true},
		{"_invalid", true},
		{".invalid", true},
		{"has space", true},
		{"has/slash", true},
		{"has:colon", true},
	}

	for _, tt := range tests {
		err := ValidateVRFName(tt.name)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateVRFName(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
		}
	}
}

func TestVRFHasIPv4(t *testing.T) {
	v := &VRF{AddressFamilies: AddressFamilyConfig{IPv4Unicast: &IPv4UnicastAF{}}}
	if !v.HasIPv4() {
		t.Error("expected HasIPv4() = true")
	}

	v2 := &VRF{}
	if v2.HasIPv4() {
		t.Error("expected HasIPv4() = false")
	}
}

func TestVRFHasIPv6(t *testing.T) {
	v := &VRF{AddressFamilies: AddressFamilyConfig{IPv6Unicast: &IPv6UnicastAF{}}}
	if !v.HasIPv6() {
		t.Error("expected HasIPv6() = true")
	}

	v2 := &VRF{}
	if v2.HasIPv6() {
		t.Error("expected HasIPv6() = false")
	}
}
