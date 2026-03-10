package dhcp4

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/config/ip"
	"github.com/veesix-networks/osvbng/pkg/dhcp"
	"github.com/veesix-networks/osvbng/pkg/provider"
)

type DHCPProvider interface {
	provider.Provider
	HandlePacket(ctx context.Context, pkt *Packet) (*Packet, error)
	ReleaseLease(mac string)
}

type BindingCounter interface {
	BindingCount() int
}

type Packet struct {
	SessionID string
	MAC       string
	SVLAN     uint16
	CVLAN     uint16
	Raw       []byte
	Resolved  *dhcp.ResolvedDHCPv4
	SwIfIndex uint32
	Interface string
	Profile   *ip.IPv4Profile
}
