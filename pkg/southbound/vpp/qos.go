package vpp

func (v *VPP) ApplyQoS(swIfIndex uint32, upMbps, downMbps int) error {
	v.logger.Debug("QoS not yet implemented", "sw_if_index", swIfIndex, "up_mbps", upMbps, "down_mbps", downMbps)
	return nil
}
