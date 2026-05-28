// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"strings"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/config/protocols"
)

func newRoutingConfForTest() *RoutingConf {
	rc := NewRoutingConf()
	rc.external.TemplateDir = "../../templates"
	return rc
}

func TestRoutingRender_BGPNeighborPassword(t *testing.T) {
	cfg := &Config{
		Protocols: protocols.ProtocolConfig{
			BGP: &protocols.BGPConfig{
				ASN: 65000,
				Neighbors: map[string]*protocols.BGPNeighbor{
					"10.0.0.2": {
						RemoteAS: 65000,
						Password: "per-neighbor-secret",
					},
				},
			},
		},
	}

	out, err := newRoutingConfForTest().GenerateConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}

	if !strings.Contains(out, "neighbor 10.0.0.2 password per-neighbor-secret") {
		t.Errorf("missing per-neighbor password line\n%s", out)
	}
}

func TestRoutingRender_BGPPeerGroupPassword(t *testing.T) {
	cfg := &Config{
		Protocols: protocols.ProtocolConfig{
			BGP: &protocols.BGPConfig{
				ASN: 65000,
				PeerGroups: map[string]*protocols.BGPPeerGroup{
					"iBGP_RR_CLIENTS": {
						RemoteAS: 65000,
						Password: "group-secret",
					},
				},
				Neighbors: map[string]*protocols.BGPNeighbor{
					"10.0.0.2": {
						PeerGroup: "iBGP_RR_CLIENTS",
					},
				},
			},
		},
	}

	out, err := newRoutingConfForTest().GenerateConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}

	if !strings.Contains(out, "neighbor iBGP_RR_CLIENTS password group-secret") {
		t.Errorf("missing peer-group password line\n%s", out)
	}
	if strings.Contains(out, "neighbor 10.0.0.2 password") {
		t.Errorf("inheriting neighbor must not emit its own password line\n%s", out)
	}
}

func TestRoutingRender_BGPNeighborAndPeerGroupPassword(t *testing.T) {
	cfg := &Config{
		Protocols: protocols.ProtocolConfig{
			BGP: &protocols.BGPConfig{
				ASN: 65000,
				PeerGroups: map[string]*protocols.BGPPeerGroup{
					"iBGP_RR_CLIENTS": {
						RemoteAS: 65000,
						Password: "group-secret",
					},
				},
				Neighbors: map[string]*protocols.BGPNeighbor{
					"10.0.0.2": {
						PeerGroup: "iBGP_RR_CLIENTS",
						Password:  "override-secret",
					},
				},
			},
		},
	}

	out, err := newRoutingConfForTest().GenerateConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}

	if !strings.Contains(out, "neighbor iBGP_RR_CLIENTS password group-secret") {
		t.Errorf("missing peer-group password line\n%s", out)
	}
	if !strings.Contains(out, "neighbor 10.0.0.2 password override-secret") {
		t.Errorf("missing per-neighbor override password line\n%s", out)
	}
}

func TestRoutingRender_BGPIPv6NeighborPassword(t *testing.T) {
	cfg := &Config{
		Protocols: protocols.ProtocolConfig{
			BGP: &protocols.BGPConfig{
				ASN: 65000,
				Neighbors: map[string]*protocols.BGPNeighbor{
					"2001:db8::1": {
						RemoteAS: 65000,
						Password: "v6-secret",
					},
				},
			},
		},
	}

	out, err := newRoutingConfForTest().GenerateConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}

	if !strings.Contains(out, "neighbor 2001:db8::1 password v6-secret") {
		t.Errorf("missing IPv6 neighbor password line\n%s", out)
	}
}

func TestRoutingRender_BGPVRFNeighborPassword(t *testing.T) {
	cfg := &Config{
		Protocols: protocols.ProtocolConfig{
			BGP: &protocols.BGPConfig{
				ASN: 65000,
				VRF: map[string]*protocols.BGPVRFConfig{
					"CUSTOMER_A": {
						RouterID: "10.0.0.1",
						Neighbors: map[string]*protocols.BGPNeighbor{
							"10.20.0.2": {
								RemoteAS: 65100,
								Password: "vrf-secret",
							},
						},
					},
				},
			},
		},
	}

	out, err := newRoutingConfForTest().GenerateConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}

	if !strings.Contains(out, "router bgp 65000 vrf CUSTOMER_A") {
		t.Fatalf("missing VRF router bgp line\n%s", out)
	}
	if !strings.Contains(out, "neighbor 10.20.0.2 password vrf-secret") {
		t.Errorf("missing VRF neighbor password line\n%s", out)
	}
}

func TestRoutingRender_OSPFInterfaceAuthMessageDigest(t *testing.T) {
	cfg := &Config{
		Protocols: protocols.ProtocolConfig{
			OSPF: &protocols.OSPFConfig{
				Enabled: true,
				Areas: map[string]*protocols.OSPFArea{
					"0.0.0.0": {
						Interfaces: map[string]*protocols.OSPFInterfaceConfig{
							"eth1": {
								Authentication: &protocols.OSPFInterfaceAuth{
									Mode:  protocols.OSPFInterfaceAuthMessageDigest,
									KeyID: 1,
									Key:   "backbone-md5",
								},
							},
						},
					},
				},
			},
		},
	}

	out, err := newRoutingConfForTest().GenerateConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}

	if !strings.Contains(out, "ip ospf authentication message-digest") {
		t.Errorf("missing per-interface authentication line\n%s", out)
	}
	if !strings.Contains(out, "ip ospf message-digest-key 1 md5 backbone-md5") {
		t.Errorf("missing message-digest-key line\n%s", out)
	}
}

func TestRoutingRender_OSPFInterfaceAuthSimple(t *testing.T) {
	cfg := &Config{
		Protocols: protocols.ProtocolConfig{
			OSPF: &protocols.OSPFConfig{
				Enabled: true,
				Areas: map[string]*protocols.OSPFArea{
					"0.0.0.0": {
						Interfaces: map[string]*protocols.OSPFInterfaceConfig{
							"eth1": {
								Authentication: &protocols.OSPFInterfaceAuth{
									Mode: protocols.OSPFInterfaceAuthSimple,
									Key:  "plain",
								},
							},
						},
					},
				},
			},
		},
	}

	out, err := newRoutingConfForTest().GenerateConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}

	if !strings.Contains(out, "ip ospf authentication\n") && !strings.Contains(out, "ip ospf authentication \n") {
		t.Errorf("missing bare authentication line\n%s", out)
	}
	if !strings.Contains(out, "ip ospf authentication-key plain") {
		t.Errorf("missing authentication-key line\n%s", out)
	}
}

func TestRoutingRender_OSPFInterfaceAuthNull(t *testing.T) {
	cfg := &Config{
		Protocols: protocols.ProtocolConfig{
			OSPF: &protocols.OSPFConfig{
				Enabled: true,
				Areas: map[string]*protocols.OSPFArea{
					"0.0.0.0": {
						Authentication: protocols.OSPFAuthMessageDigest,
						Interfaces: map[string]*protocols.OSPFInterfaceConfig{
							"eth1": {
								Authentication: &protocols.OSPFInterfaceAuth{
									Mode: protocols.OSPFInterfaceAuthNull,
								},
							},
						},
					},
				},
			},
		},
	}

	out, err := newRoutingConfForTest().GenerateConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}

	if !strings.Contains(out, "ip ospf authentication null") {
		t.Errorf("missing null authentication override\n%s", out)
	}
}

func TestRoutingRender_OSPFAreaAuthSimple(t *testing.T) {
	cfg := &Config{
		Protocols: protocols.ProtocolConfig{
			OSPF: &protocols.OSPFConfig{
				Enabled: true,
				Areas: map[string]*protocols.OSPFArea{
					"0.0.0.0": {
						Authentication: protocols.OSPFAuthSimple,
					},
				},
			},
		},
	}

	out, err := newRoutingConfForTest().GenerateConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}

	if !strings.Contains(out, "area 0.0.0.0 authentication\n") && !strings.Contains(out, "area 0.0.0.0 authentication \n") {
		t.Errorf("missing bare area authentication line\n%s", out)
	}
	if strings.Contains(out, "area 0.0.0.0 authentication message-digest") {
		t.Errorf("simple mode should not emit message-digest keyword\n%s", out)
	}
}

func TestRoutingRender_OSPF6InterfaceAuth(t *testing.T) {
	cfg := &Config{
		Protocols: protocols.ProtocolConfig{
			OSPF6: &protocols.OSPF6Config{
				Enabled: true,
				Areas: map[string]*protocols.OSPF6Area{
					"0.0.0.0": {
						Interfaces: map[string]*protocols.OSPF6InterfaceConfig{
							"eth1": {
								Authentication: &protocols.OSPF6InterfaceAuth{
									KeyID:    10,
									HashAlgo: protocols.OSPF6HashHMACSHA256,
									Key:      "backbone-v6",
								},
							},
						},
					},
				},
			},
		},
	}

	out, err := newRoutingConfForTest().GenerateConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}

	if !strings.Contains(out, "ipv6 ospf6 authentication key-id 10 hash-algo hmac-sha-256 key backbone-v6") {
		t.Errorf("missing ospfv3 authentication trailer line\n%s", out)
	}
}

func TestRoutingRender_BGPNoPasswordBackwardCompat(t *testing.T) {
	cfg := &Config{
		Protocols: protocols.ProtocolConfig{
			BGP: &protocols.BGPConfig{
				ASN: 65000,
				Neighbors: map[string]*protocols.BGPNeighbor{
					"10.0.0.2": {
						RemoteAS: 65000,
					},
				},
			},
		},
	}

	out, err := newRoutingConfForTest().GenerateConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}

	if strings.Contains(out, "password") {
		t.Errorf("unexpected password line in config without Password field\n%s", out)
	}
}

func TestRoutingRender_OSPFVRFInstance(t *testing.T) {
	cfg := &Config{
		Protocols: protocols.ProtocolConfig{
			OSPF: &protocols.OSPFConfig{
				Enabled:             true,
				RouterID:            "10.0.0.1",
				LogAdjacencyChanges: true,
				Areas: map[string]*protocols.OSPFArea{
					"0.0.0.0": {
						Interfaces: map[string]*protocols.OSPFInterfaceConfig{
							"eth0": {Network: protocols.OSPFNetworkPointToPoint},
						},
					},
				},
				VRF: map[string]*protocols.OSPFVRFConfig{
					"MGMT-VRF": {
						RouterID:            "10.99.0.1",
						LogAdjacencyChanges: true,
						MaximumPaths:        4,
						Areas: map[string]*protocols.OSPFArea{
							"0.0.0.0": {
								Interfaces: map[string]*protocols.OSPFInterfaceConfig{
									"eth1.99": {
										Network: protocols.OSPFNetworkPointToPoint,
										Authentication: &protocols.OSPFInterfaceAuth{
											Mode:  protocols.OSPFInterfaceAuthMessageDigest,
											KeyID: 1,
											Key:   "vrf-secret",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	out, err := newRoutingConfForTest().GenerateConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}

	checks := []string{
		"router ospf",
		"router ospf vrf MGMT-VRF",
		"ospf router-id 10.99.0.1",
		"maximum-paths 4",
		"interface eth1.99",
		"ip ospf area 0.0.0.0",
		"ip ospf network point-to-point",
		"ip ospf authentication message-digest",
		"ip ospf message-digest-key 1 md5 vrf-secret",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output\n%s", want, out)
		}
	}
}

func TestRoutingRender_OSPFVRFOnlyNoGlobal(t *testing.T) {
	cfg := &Config{
		Protocols: protocols.ProtocolConfig{
			OSPF: &protocols.OSPFConfig{
				Enabled: false,
				VRF: map[string]*protocols.OSPFVRFConfig{
					"MGMT-VRF": {
						RouterID: "10.99.0.1",
						Areas: map[string]*protocols.OSPFArea{
							"0.0.0.0": {
								Interfaces: map[string]*protocols.OSPFInterfaceConfig{
									"eth1.99": {Network: protocols.OSPFNetworkPointToPoint},
								},
							},
						},
					},
				},
			},
		},
	}

	out, err := newRoutingConfForTest().GenerateConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}

	if !strings.Contains(out, "router ospf vrf MGMT-VRF") {
		t.Errorf("missing VRF block in output\n%s", out)
	}
	if strings.Contains(out, "\nrouter ospf\n") {
		t.Errorf("global router ospf block present when Enabled=false\n%s", out)
	}
}

func TestRoutingRender_OSPF6VRFInstance(t *testing.T) {
	cfg := &Config{
		Protocols: protocols.ProtocolConfig{
			OSPF6: &protocols.OSPF6Config{
				Enabled:  true,
				RouterID: "10.0.0.1",
				Areas: map[string]*protocols.OSPF6Area{
					"0.0.0.0": {
						Interfaces: map[string]*protocols.OSPF6InterfaceConfig{
							"eth0": {Network: protocols.OSPF6NetworkPointToPoint},
						},
					},
				},
				VRF: map[string]*protocols.OSPF6VRFConfig{
					"MGMT-VRF": {
						RouterID: "10.99.0.1",
						Areas: map[string]*protocols.OSPF6Area{
							"0.0.0.0": {
								Interfaces: map[string]*protocols.OSPF6InterfaceConfig{
									"eth1.99": {Network: protocols.OSPF6NetworkPointToPoint},
								},
							},
						},
					},
				},
			},
		},
	}

	out, err := newRoutingConfForTest().GenerateConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}

	checks := []string{
		"router ospf6",
		"router ospf6 vrf MGMT-VRF",
		"ospf6 router-id 10.99.0.1",
		"interface eth1.99",
		"ipv6 ospf6 area 0.0.0.0",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output\n%s", want, out)
		}
	}
}


func buildOSPFVRFTestConfig(declaredVRF, ospfVRF string) *Config {
	return &Config{
		Interfaces: map[string]*interfaces.InterfaceConfig{
			"eth2": {
				Name: "eth2",
				VRF:  declaredVRF,
			},
		},
		Protocols: protocols.ProtocolConfig{
			OSPF: &protocols.OSPFConfig{
				Enabled: true,
				VRF: map[string]*protocols.OSPFVRFConfig{
					ospfVRF: {
						Areas: map[string]*protocols.OSPFArea{
							"0.0.0.0": {
								Interfaces: map[string]*protocols.OSPFInterfaceConfig{
									"eth2": {},
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestRoutingValidate_OSPFVRFInterfaceMismatchRejected(t *testing.T) {
	cfg := buildOSPFVRFTestConfig("MGMT-VRF", "OTHER-VRF")
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "must match") {
		t.Fatalf("expected VRF-mismatch error, got %v", err)
	}
}

func TestRoutingValidate_OSPFVRFInterfaceInBothRejected(t *testing.T) {
	cfg := buildOSPFVRFTestConfig("MGMT-VRF", "MGMT-VRF")
	cfg.Protocols.OSPF.Areas = map[string]*protocols.OSPFArea{
		"0.0.0.0": {
			Interfaces: map[string]*protocols.OSPFInterfaceConfig{
				"eth2": {},
			},
		},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "also under global") {
		t.Fatalf("expected dual-block error, got %v", err)
	}
}

func TestRoutingValidate_OSPFVRFInterfaceMatchAccepted(t *testing.T) {
	cfg := buildOSPFVRFTestConfig("MGMT-VRF", "MGMT-VRF")
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected validation success, got %v", err)
	}
}
