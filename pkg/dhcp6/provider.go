package dhcp6

import (
	"context"
	"net"

	"github.com/veesix-networks/osvbng/pkg/config/ip"
	"github.com/veesix-networks/osvbng/pkg/dhcp"
	"github.com/veesix-networks/osvbng/pkg/provider"
)

type DHCPProvider interface {
	provider.Provider
	HandlePacket(ctx context.Context, pkt *Packet) (*Packet, error)
	ReleaseLease(duid []byte)
}

type BindingCounter interface {
	BindingCount() int
}

type Packet struct {
	SessionID string
	MAC       string
	SVLAN     uint16
	CVLAN     uint16
	DUID      []byte
	Raw       []byte
	Resolved  *dhcp.ResolvedDHCPv6
	SwIfIndex uint32
	Interface string
	PeerAddr  net.IP
	Profile   *ip.IPv6Profile
}
