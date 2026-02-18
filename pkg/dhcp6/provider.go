package dhcp6

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/dhcp"
	"github.com/veesix-networks/osvbng/pkg/provider"
)

type DHCPProvider interface {
	provider.Provider
	HandlePacket(ctx context.Context, pkt *Packet) (*Packet, error)
}

type Packet struct {
	SessionID string
	MAC       string
	SVLAN     uint16
	CVLAN     uint16
	DUID      []byte
	Raw       []byte
	Resolved  *dhcp.ResolvedDHCPv6
}
