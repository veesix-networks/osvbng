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

	tableID, err := m.ResolveVRF("")
	if err != nil {
		t.Fatalf("ResolveVRF(\"\") error: %v", err)
	}
	if tableID != 0 {
		t.Fatalf("expected 0 for empty VRF, got %d", tableID)
	}
}

func TestResolveVRFNotFound(t *testing.T) {
	m := &Manager{
		vrfs: make(map[string]*vrfEntry),
	}

	_, err := m.ResolveVRF("nonexistent")
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

	tableID, err := m.ResolveVRF("customers")
	if err != nil {
		t.Fatalf("ResolveVRF(\"customers\") error: %v", err)
	}
	if tableID != 100 {
		t.Fatalf("expected 100, got %d", tableID)
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
