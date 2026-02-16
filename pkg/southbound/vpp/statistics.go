package vpp

import (
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func (v *VPP) GetDataplaneStats() (*southbound.DataplaneStats, error) {
	return v.statsClient.GetAllStats()
}


func (v *VPP) GetSystemStats() (*southbound.SystemStats, error) {
	return v.statsClient.GetSystemStats()
}


func (v *VPP) GetMemoryStats() ([]southbound.MemoryStats, error) {
	return v.statsClient.GetMemoryStats()
}


func (v *VPP) GetInterfaceStats() ([]southbound.InterfaceStats, error) {
	return v.statsClient.GetInterfaceStats()
}


func (v *VPP) GetNodeStats() ([]southbound.NodeStats, error) {
	return v.statsClient.GetNodeStats()
}


func (v *VPP) GetErrorStats() ([]southbound.ErrorStats, error) {
	return v.statsClient.GetErrorStats()
}


func (v *VPP) GetBufferStats() ([]southbound.BufferStats, error) {
	return v.statsClient.GetBufferStats()
}


