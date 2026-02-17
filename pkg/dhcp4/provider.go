package dhcp4

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/allocator"
	"github.com/veesix-networks/osvbng/pkg/provider"
)

type DHCPProvider interface {
	provider.Provider
	HandlePacket(ctx context.Context, pkt *Packet) (*Packet, error)
}

type Packet struct {
	Context   *allocator.Context
	SessionID string
	MAC       string
	SVLAN     uint16
	CVLAN     uint16
	Raw       []byte
}
