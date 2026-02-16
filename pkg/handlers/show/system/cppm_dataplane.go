package system

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func init() {
	show.RegisterFactory(NewCPPMDataplaneHandler)
}

type CPPMDataplaneHandler struct {
	southbound southbound.Southbound
}

func NewCPPMDataplaneHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &CPPMDataplaneHandler{
		southbound: deps.Southbound,
	}
}

var protocolNames = map[uint8]string{
	0: "dhcpv4",
	1: "dhcpv6",
	2: "arp",
	3: "pppoe-disc",
	4: "pppoe-sess",
	5: "ipv6-nd",
	6: "l2tp",
}

type DataplaneCPPMStats struct {
	Protocol       string  `json:"protocol"`
	PacketsPunted  uint64  `json:"packets_punted"`
	PacketsDropped uint64  `json:"packets_dropped"`
	PacketsPoliced uint64  `json:"packets_policed"`
	PolicerRate    float64 `json:"policer_rate"`
	PolicerBurst   uint32  `json:"policer_burst"`
}

func (h *CPPMDataplaneHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	stats, err := h.southbound.GetPuntStats()
	if err != nil {
		return nil, err
	}

	result := make([]DataplaneCPPMStats, 0, len(stats))
	for _, s := range stats {
		name := protocolNames[s.Protocol]
		if name == "" {
			name = "unknown"
		}
		result = append(result, DataplaneCPPMStats{
			Protocol:       name,
			PacketsPunted:  s.PacketsPunted,
			PacketsDropped: s.PacketsDropped,
			PacketsPoliced: s.PacketsPoliced,
			PolicerRate:    s.PolicerRate,
			PolicerBurst:   s.PolicerBurst,
		})
	}

	return result, nil
}

func (h *CPPMDataplaneHandler) PathPattern() paths.Path {
	return paths.SystemCPPMDataplane
}

func (h *CPPMDataplaneHandler) Dependencies() []paths.Path {
	return nil
}
