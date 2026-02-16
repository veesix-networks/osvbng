package southbound

type MPLS interface {
	CreateMPLSTable() error
	EnableMPLS(swIfIndex uint32) error

	GetMPLSRoutes() ([]*MPLSRouteEntry, error)
	GetMPLSInterfaces() ([]*MPLSInterfaceInfo, error)
}
