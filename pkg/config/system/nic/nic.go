package nic

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Vendor interface {
	Name() string
	Match(vendorID string) bool
	BindStrategy() BindStrategy
}

type BindStrategy int

const (
	BindStrategyVFIO BindStrategy = iota
	BindStrategyBifurcated
	BindStrategyUIO
)

func (s BindStrategy) String() string {
	switch s {
	case BindStrategyVFIO:
		return "vfio-pci"
	case BindStrategyBifurcated:
		return "bifurcated"
	case BindStrategyUIO:
		return "uio_pci_generic"
	default:
		return "unknown"
	}
}

var registeredVendors []Vendor

func Register(v Vendor) {
	registeredVendors = append(registeredVendors, v)
}

func DetectVendor(pci string) (Vendor, error) {
	vendorID, err := GetPCIVendorID(pci)
	if err != nil {
		return nil, err
	}

	for _, vendor := range registeredVendors {
		if vendor.Match(vendorID) {
			return vendor, nil
		}
	}

	return nil, fmt.Errorf("no matching vendor for %s", vendorID)
}

func GetPCIVendorID(pci string) (string, error) {
	sysfsPath := filepath.Join("/sys/bus/pci/devices", pci)
	vendorBytes, err := os.ReadFile(filepath.Join(sysfsPath, "vendor"))
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(strings.TrimSpace(string(vendorBytes)), "0x"), nil
}

func GetPCIDeviceID(pci string) (string, error) {
	sysfsPath := filepath.Join("/sys/bus/pci/devices", pci)
	deviceBytes, err := os.ReadFile(filepath.Join(sysfsPath, "device"))
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(strings.TrimSpace(string(deviceBytes)), "0x"), nil
}
