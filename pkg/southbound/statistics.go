package southbound

type Statistics interface {
	GetDataplaneStats() (*DataplaneStats, error)
	GetSystemStats() (*SystemStats, error)
	GetMemoryStats() ([]MemoryStats, error)
	GetInterfaceStats() ([]InterfaceStats, error)
	GetNodeStats() ([]NodeStats, error)
	GetErrorStats() ([]ErrorStats, error)
	GetBufferStats() ([]BufferStats, error)
}
