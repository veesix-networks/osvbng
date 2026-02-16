package southbound

type Addressing interface {
	AddAdjacencyWithRewrite(ipAddr string, swIfIndex uint32, rewrite []byte) (uint32, error)
	UnlockAdjacency(adjIndex uint32) error
	AddHostRoute(ipAddr string, adjIndex uint32, fibID uint32, swIfIndex uint32) error
	DeleteHostRoute(ipAddr string, fibID uint32) error

	AddAdjacencyWithRewriteAsync(ipAddr string, swIfIndex uint32, rewrite []byte, callback func(adjIndex uint32, err error))
	AddHostRouteAsync(ipAddr string, adjIndex uint32, fibID uint32, swIfIndex uint32, callback func(error))
	DeleteHostRouteAsync(ipAddr string, fibID uint32, callback func(error))

	BuildL2Rewrite(dstMAC, srcMAC string, outerVLAN, innerVLAN uint16) []byte
	GetFIBIDForInterface(swIfIndex uint32) (uint32, error)
}
