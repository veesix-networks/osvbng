package config

import (
	"fmt"
	"strconv"
	"strings"
)

func ParseVLANRange(vlanStr string) ([]uint16, error) {
	vlanStr = strings.TrimSpace(vlanStr)

	if strings.Contains(vlanStr, "-") {
		parts := strings.Split(vlanStr, "-")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid VLAN range format: %s", vlanStr)
		}

		start, err := strconv.ParseUint(strings.TrimSpace(parts[0]), 10, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid start VLAN: %w", err)
		}

		end, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid end VLAN: %w", err)
		}

		if start > end {
			return nil, fmt.Errorf("invalid VLAN range: start (%d) > end (%d)", start, end)
		}

		if start == 0 || end == 0 {
			return nil, fmt.Errorf("VLAN 0 not allowed (untagged not supported)")
		}

		if end > 4094 {
			return nil, fmt.Errorf("VLAN %d exceeds maximum (4094)", end)
		}

		vlans := make([]uint16, 0, end-start+1)
		for i := start; i <= end; i++ {
			vlans = append(vlans, uint16(i))
		}

		return vlans, nil
	}

	vlan, err := strconv.ParseUint(vlanStr, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid VLAN: %w", err)
	}

	if vlan == 0 {
		return nil, fmt.Errorf("VLAN 0 not allowed (untagged not supported)")
	}

	if vlan > 4094 {
		return nil, fmt.Errorf("VLAN %d exceeds maximum (4094)", vlan)
	}

	return []uint16{uint16(vlan)}, nil
}

func ParseCVLAN(cvlanStr string) (bool, uint16, error) {
	cvlanStr = strings.TrimSpace(cvlanStr)

	if strings.ToLower(cvlanStr) == "any" {
		return true, 0, nil
	}

	cvlan, err := strconv.ParseUint(cvlanStr, 10, 16)
	if err != nil {
		return false, 0, fmt.Errorf("invalid C-VLAN: %w", err)
	}

	if cvlan > 4094 {
		return false, 0, fmt.Errorf("C-VLAN %d exceeds maximum (4094)", cvlan)
	}

	return false, uint16(cvlan), nil
}
