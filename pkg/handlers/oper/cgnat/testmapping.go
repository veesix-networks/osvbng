// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"context"
	"encoding/json"
	"fmt"
	"net"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper/paths"
)

func init() {
	oper.RegisterFactory(func(d *deps.OperDeps) oper.OperHandler {
		return &TestMappingHandler{deps: d}
	})
}

type TestMappingHandler struct {
	deps *deps.OperDeps
}

type TestMappingRequest struct {
	Pool      string `json:"pool"`
	Direction string `json:"direction"`
	InsideIP  string `json:"inside_ip,omitempty"`
	OutsideIP string `json:"outside_ip,omitempty"`
	Port      uint16 `json:"port,omitempty"`
}

type TestMappingResponse struct {
	Direction string `json:"direction"`
	InsideIP  string `json:"inside_ip,omitempty"`
	OutsideIP string `json:"outside_ip,omitempty"`
	PortStart uint16 `json:"port_start,omitempty"`
	PortEnd   uint16 `json:"port_end,omitempty"`
}

func (h *TestMappingHandler) Execute(_ context.Context, req *oper.Request) (interface{}, error) {
	if h.deps.CGNAT == nil {
		return nil, fmt.Errorf("CGNAT not configured")
	}

	var tmReq TestMappingRequest
	if err := json.Unmarshal(req.Body, &tmReq); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	cfg, err := h.deps.CGNAT.GetRunningConfig()
	if err != nil || cfg == nil {
		return nil, fmt.Errorf("no CGNAT configuration")
	}

	poolCfg, ok := cfg.Pools[tmReq.Pool]
	if !ok {
		return nil, fmt.Errorf("pool %s not found", tmReq.Pool)
	}

	if poolCfg.GetMode() != "deterministic" {
		return nil, fmt.Errorf("test-mapping only works with deterministic pools")
	}

	if len(poolCfg.InsidePrefixes) == 0 || len(poolCfg.OutsideAddresses) == 0 {
		return nil, fmt.Errorf("pool %s has no inside/outside prefixes configured", tmReq.Pool)
	}

	_, insideNet, _ := net.ParseCIDR(poolCfg.InsidePrefixes[0].Prefix)
	_, outsideNet, _ := net.ParseCIDR(poolCfg.OutsideAddresses[0])

	insideOnes, _ := insideNet.Mask.Size()
	outsideOnes, _ := outsideNet.Mask.Size()
	insideCount := uint32(1) << uint(32-insideOnes)
	outsideCount := uint32(1) << uint(32-outsideOnes)
	sharingRatio := insideCount / outsideCount
	if sharingRatio == 0 {
		sharingRatio = 1
	}
	portRangeStart := poolCfg.GetPortRangeStart()
	portRangeEnd := poolCfg.GetPortRangeEnd()
	usablePorts := uint32(portRangeEnd) - uint32(portRangeStart) + 1
	portsPerHost := usablePorts / sharingRatio

	insideBase := ipToU32(insideNet.IP.To4())
	outsideBase := ipToU32(outsideNet.IP.To4())

	switch tmReq.Direction {
	case "forward":
		ip := net.ParseIP(tmReq.InsideIP)
		if ip == nil {
			return nil, fmt.Errorf("invalid inside_ip")
		}
		hostOffset := ipToU32(ip.To4()) - insideBase
		if hostOffset >= insideCount {
			return nil, fmt.Errorf("inside IP not in pool range")
		}
		outsideOffset := hostOffset / sharingRatio
		portIndex := hostOffset % sharingRatio
		outsideIP := u32ToIP(outsideBase + outsideOffset)
		pStart := portRangeStart + uint16(portsPerHost*portIndex)
		pEnd := pStart + uint16(portsPerHost) - 1

		return &TestMappingResponse{
			Direction: "forward",
			InsideIP:  ip.String(),
			OutsideIP: outsideIP.String(),
			PortStart: pStart,
			PortEnd:   pEnd,
		}, nil

	case "reverse":
		ip := net.ParseIP(tmReq.OutsideIP)
		if ip == nil {
			return nil, fmt.Errorf("invalid outside_ip")
		}
		outsideOffset := ipToU32(ip.To4()) - outsideBase
		if outsideOffset >= outsideCount {
			return nil, fmt.Errorf("outside IP not in pool range")
		}
		if tmReq.Port < portRangeStart || tmReq.Port > portRangeEnd {
			return nil, fmt.Errorf("port not in range %d-%d", portRangeStart, portRangeEnd)
		}
		portOffset := uint32(tmReq.Port-portRangeStart) / portsPerHost
		hostOffset := (outsideOffset * sharingRatio) + portOffset
		if hostOffset >= insideCount {
			return nil, fmt.Errorf("computed host offset out of range")
		}
		insideIP := u32ToIP(insideBase + hostOffset)

		return &TestMappingResponse{
			Direction: "reverse",
			InsideIP:  insideIP.String(),
			OutsideIP: ip.String(),
			PortStart: tmReq.Port,
		}, nil

	default:
		return nil, fmt.Errorf("direction must be 'forward' or 'reverse'")
	}
}

func (h *TestMappingHandler) PathPattern() paths.Path {
	return paths.CGNATTestMapping
}

func (h *TestMappingHandler) Dependencies() []paths.Path {
	return nil
}

func ipToU32(ip net.IP) uint32 {
	ip4 := ip.To4()
	return uint32(ip4[0])<<24 | uint32(ip4[1])<<16 | uint32(ip4[2])<<8 | uint32(ip4[3])
}

func u32ToIP(v uint32) net.IP {
	return net.IPv4(byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}
