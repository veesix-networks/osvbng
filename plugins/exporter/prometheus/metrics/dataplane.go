package metrics

import (
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	RegisterMetricSingle[southbound.SystemStats](paths.SystemDataplaneSystem)
	RegisterMetricMulti[southbound.MemoryStats](paths.SystemDataplaneMemory)
	RegisterMetricMulti[southbound.InterfaceStats](paths.SystemDataplaneInterfaces)
	RegisterMetricMulti[southbound.NodeStats](paths.SystemDataplaneNodes)
	RegisterMetricMulti[southbound.ErrorStats](paths.SystemDataplaneErrors)
	RegisterMetricMulti[southbound.BufferStats](paths.SystemDataplaneBuffers)
}
