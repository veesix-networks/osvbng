package vrfmgr

import (
	"testing"
)

func TestAllocateTableID(t *testing.T) {
	m := &Manager{
		vrfs: make(map[string]*vrfEntry),
	}

	id, err := m.allocateTableID()
	if err != nil {
		t.Fatalf("allocateTableID() error: %v", err)
	}
	if id != tableIDMin {
		t.Fatalf("expected first ID %d, got %d", tableIDMin, id)
	}

	m.vrfs["test"] = &vrfEntry{TableID: tableIDMin}
	id, err = m.allocateTableID()
	if err != nil {
		t.Fatalf("allocateTableID() error: %v", err)
	}
	if id != tableIDMin+1 {
		t.Fatalf("expected second ID %d, got %d", tableIDMin+1, id)
	}
}

func TestAllocateTableIDSkipsUsed(t *testing.T) {
	m := &Manager{
		vrfs: make(map[string]*vrfEntry),
	}

	for i := tableIDMin; i < tableIDMin+10; i++ {
		m.vrfs[string(rune(i))] = &vrfEntry{TableID: i}
	}

	id, err := m.allocateTableID()
	if err != nil {
		t.Fatalf("allocateTableID() error: %v", err)
	}
	if id != tableIDMin+10 {
		t.Fatalf("expected ID %d, got %d", tableIDMin+10, id)
	}
}

func TestResolveVRFEmpty(t *testing.T) {
	m := &Manager{
		vrfs: make(map[string]*vrfEntry),
	}

	tableID, ipv4, ipv6, err := m.ResolveVRF("")
	if err != nil {
		t.Fatalf("ResolveVRF(\"\") error: %v", err)
	}
	if tableID != 0 {
		t.Fatalf("expected 0 for empty VRF, got %d", tableID)
	}
	if ipv4 || ipv6 {
		t.Fatalf("expected false/false for empty VRF, got %v/%v", ipv4, ipv6)
	}
}

func TestResolveVRFNotFound(t *testing.T) {
	m := &Manager{
		vrfs: make(map[string]*vrfEntry),
	}

	_, _, _, err := m.ResolveVRF("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent VRF")
	}
}

func TestResolveVRFFound(t *testing.T) {
	m := &Manager{
		vrfs: map[string]*vrfEntry{
			"customers": {Name: "customers", TableID: 100, IPv4: true, IPv6: true},
		},
	}

	tableID, ipv4, ipv6, err := m.ResolveVRF("customers")
	if err != nil {
		t.Fatalf("ResolveVRF(\"customers\") error: %v", err)
	}
	if tableID != 100 {
		t.Fatalf("expected 100, got %d", tableID)
	}
	if !ipv4 || !ipv6 {
		t.Fatalf("expected true/true, got %v/%v", ipv4, ipv6)
	}
}

func TestResolveVRFAddressFamilies(t *testing.T) {
	m := &Manager{
		vrfs: map[string]*vrfEntry{
			"v4only": {Name: "v4only", TableID: 200, IPv4: true, IPv6: false},
			"v6only": {Name: "v6only", TableID: 201, IPv4: false, IPv6: true},
		},
	}

	_, ipv4, ipv6, err := m.ResolveVRF("v4only")
	if err != nil {
		t.Fatalf("ResolveVRF(\"v4only\") error: %v", err)
	}
	if !ipv4 || ipv6 {
		t.Fatalf("v4only: expected true/false, got %v/%v", ipv4, ipv6)
	}

	_, ipv4, ipv6, err = m.ResolveVRF("v6only")
	if err != nil {
		t.Fatalf("ResolveVRF(\"v6only\") error: %v", err)
	}
	if ipv4 || !ipv6 {
		t.Fatalf("v6only: expected false/true, got %v/%v", ipv4, ipv6)
	}
}

func TestGetVRFs(t *testing.T) {
	m := &Manager{
		vrfs: map[string]*vrfEntry{
			"vrf-a": {Name: "vrf-a", TableID: 100, IPv4: true, IPv6: false},
			"vrf-b": {Name: "vrf-b", TableID: 101, IPv4: true, IPv6: true},
		},
	}

	vrfs := m.GetVRFs()
	if len(vrfs) != 2 {
		t.Fatalf("expected 2 VRFs, got %d", len(vrfs))
	}

	found := make(map[string]bool)
	for _, v := range vrfs {
		found[v.Name] = true
		if v.Name == "vrf-a" {
			if v.TableId != 100 {
				t.Errorf("vrf-a: expected tableId 100, got %d", v.TableId)
			}
			if v.AddressFamilies.IPv4Unicast == nil {
				t.Error("vrf-a: expected IPv4 enabled")
			}
			if v.AddressFamilies.IPv6Unicast != nil {
				t.Error("vrf-a: expected IPv6 disabled")
			}
		}
		if v.Name == "vrf-b" {
			if v.TableId != 101 {
				t.Errorf("vrf-b: expected tableId 101, got %d", v.TableId)
			}
			if v.AddressFamilies.IPv4Unicast == nil {
				t.Error("vrf-b: expected IPv4 enabled")
			}
			if v.AddressFamilies.IPv6Unicast == nil {
				t.Error("vrf-b: expected IPv6 enabled")
			}
		}
	}

	if !found["vrf-a"] || !found["vrf-b"] {
		t.Errorf("missing VRFs: %v", found)
	}
}
