package southbound

type Tables interface {
	GetIPTables() ([]*IPTableInfo, error)
	GetIPMTables() ([]*IPMTableInfo, error)
	AddIPTable(tableID uint32, isIPv6 bool, name string) error
	DelIPTable(tableID uint32, isIPv6 bool) error
	GetNextAvailableGlobalTableId() (uint32, error)
}
