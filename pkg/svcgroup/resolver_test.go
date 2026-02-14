package svcgroup

import (
	"testing"

	"github.com/veesix-networks/osvbng/pkg/config/servicegroup"
)

func TestSetDelete(t *testing.T) {
	r := New()

	cfg := &servicegroup.Config{VRF: "cgnat", Unnumbered: "loop101"}
	r.Set("residential", cfg)

	got := r.Get("residential")
	if got == nil {
		t.Fatal("expected config, got nil")
	}
	if got.VRF != "cgnat" {
		t.Errorf("expected VRF cgnat, got %s", got.VRF)
	}

	r.Delete("residential")
	if r.Get("residential") != nil {
		t.Error("expected nil after delete")
	}
}

func TestGetAll(t *testing.T) {
	r := New()
	r.Set("a", &servicegroup.Config{VRF: "v1"})
	r.Set("b", &servicegroup.Config{VRF: "v2"})

	all := r.GetAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(all))
	}
}

func TestResolveDefaultOnly(t *testing.T) {
	r := New()
	r.Set("default-sg", &servicegroup.Config{
		VRF:        "cgnat",
		Unnumbered: "loop101",
		URPF:       "strict",
	})

	result := r.Resolve("", "default-sg", nil)
	if result.VRF != "cgnat" {
		t.Errorf("expected VRF cgnat, got %s", result.VRF)
	}
	if result.Unnumbered != "loop101" {
		t.Errorf("expected Unnumbered loop101, got %s", result.Unnumbered)
	}
	if result.Name != "default-sg" {
		t.Errorf("expected ServiceGroup default-sg, got %s", result.Name)
	}
}

func TestResolveAAAServiceGroupOverridesDefault(t *testing.T) {
	r := New()
	r.Set("default-sg", &servicegroup.Config{
		VRF:        "default-vrf",
		Unnumbered: "loop100",
	})
	r.Set("premium", &servicegroup.Config{
		VRF:        "premium-vrf",
		Unnumbered: "loop200",
		URPF:       "strict",
	})

	result := r.Resolve("premium", "default-sg", nil)
	if result.VRF != "premium-vrf" {
		t.Errorf("expected VRF premium-vrf, got %s", result.VRF)
	}
	if result.Unnumbered != "loop200" {
		t.Errorf("expected Unnumbered loop200, got %s", result.Unnumbered)
	}
	if result.URPF != "strict" {
		t.Errorf("expected URPF strict, got %s", result.URPF)
	}
	if result.Name != "premium" {
		t.Errorf("expected ServiceGroup premium, got %s", result.Name)
	}
}

func TestResolvePerFieldAAAOverrides(t *testing.T) {
	r := New()
	r.Set("residential", &servicegroup.Config{
		VRF:        "cgnat",
		Unnumbered: "loop101",
		QoS: &servicegroup.QoSConfig{
			UploadRate:   1000000000,
			DownloadRate: 1000000000,
		},
	})

	attrs := map[string]interface{}{
		"vrf":               "enterprise",
		"qos.download-rate": "10000000000",
	}

	result := r.Resolve("residential", "", attrs)
	if result.VRF != "enterprise" {
		t.Errorf("expected VRF enterprise, got %s", result.VRF)
	}
	if result.Unnumbered != "loop101" {
		t.Errorf("expected Unnumbered loop101 (from service group), got %s", result.Unnumbered)
	}
	if result.UploadRate != 1000000000 {
		t.Errorf("expected UploadRate 1000000000, got %d", result.UploadRate)
	}
	if result.DownloadRate != 10000000000 {
		t.Errorf("expected DownloadRate 10000000000, got %d", result.DownloadRate)
	}
}

func TestResolveUnknownServiceGroup(t *testing.T) {
	r := New()

	result := r.Resolve("nonexistent", "", nil)
	if result.VRF != "" {
		t.Errorf("expected empty VRF, got %s", result.VRF)
	}
	if result.Name != "" {
		t.Errorf("expected empty ServiceGroup, got %s", result.Name)
	}
}

func TestResolveEmptyNames(t *testing.T) {
	r := New()

	result := r.Resolve("", "", nil)
	if result != (ServiceGroup{}) {
		t.Errorf("expected zero-value ServiceGroup, got %+v", result)
	}
}

func TestResolveACLOverrides(t *testing.T) {
	r := New()
	r.Set("sg1", &servicegroup.Config{
		ACL: &servicegroup.ACLConfig{
			Ingress: "default-in",
			Egress:  "default-out",
		},
	})

	attrs := map[string]interface{}{
		"acl.ingress": "custom-in",
	}

	result := r.Resolve("sg1", "", attrs)
	if result.ACLIngress != "custom-in" {
		t.Errorf("expected ACLIngress custom-in, got %s", result.ACLIngress)
	}
	if result.ACLEgress != "default-out" {
		t.Errorf("expected ACLEgress default-out, got %s", result.ACLEgress)
	}
}
