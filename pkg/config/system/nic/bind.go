package nic

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/veesix-networks/osvbng/pkg/logger"
)

type Device struct {
	PCI  string
	Name string
}

func BindDevices(devices []Device) error {
	for _, dev := range devices {
		pci := dev.PCI
		sysfsPath := filepath.Join("/sys/bus/pci/devices", pci)

		if _, err := os.Stat(sysfsPath); os.IsNotExist(err) {
			logger.Log.Warn("Device not found, skipping", "pci", pci)
			continue
		}

		vendor, err := DetectVendor(pci)
		if err != nil {
			logger.Log.Warn("Failed to detect vendor", "pci", pci, "error", err)
			continue
		}

		strategy := vendor.BindStrategy()
		logger.Log.Info("Detected NIC vendor", "pci", pci, "vendor", vendor.Name(), "strategy", strategy.String())

		switch strategy {
		case BindStrategyBifurcated:
			logger.Log.Info("Bifurcated driver, no binding required", "pci", pci)
			continue
		case BindStrategyVFIO:
			if err := bindToVFIO(pci); err != nil {
				logger.Log.Error("Failed to bind to vfio-pci", "pci", pci, "error", err)
			}
		case BindStrategyUIO:
			if err := bindToUIO(pci); err != nil {
				logger.Log.Error("Failed to bind to uio_pci_generic", "pci", pci, "error", err)
			}
		}
	}

	return nil
}

func bindToVFIO(pci string) error {
	exec.Command("modprobe", "vfio-pci").Run()

	sysfsPath := filepath.Join("/sys/bus/pci/devices", pci)

	driverLink, err := os.Readlink(filepath.Join(sysfsPath, "driver"))
	if err == nil && filepath.Base(driverLink) == "vfio-pci" {
		logger.Log.Info("Device already bound to vfio-pci", "pci", pci)
		return nil
	}

	if err == nil {
		unbindPath := filepath.Join(sysfsPath, "driver", "unbind")
		os.WriteFile(unbindPath, []byte(pci), 0200)
	}

	vendorID, _ := GetPCIVendorID(pci)
	deviceID, _ := GetPCIDeviceID(pci)

	newID := fmt.Sprintf("%s %s", vendorID, deviceID)
	os.WriteFile("/sys/bus/pci/drivers/vfio-pci/new_id", []byte(newID), 0200)
	os.WriteFile("/sys/bus/pci/drivers/vfio-pci/bind", []byte(pci), 0200)

	logger.Log.Info("Bound device to vfio-pci", "pci", pci)
	return nil
}

func bindToUIO(pci string) error {
	exec.Command("modprobe", "uio_pci_generic").Run()

	sysfsPath := filepath.Join("/sys/bus/pci/devices", pci)

	driverLink, err := os.Readlink(filepath.Join(sysfsPath, "driver"))
	if err == nil && filepath.Base(driverLink) == "uio_pci_generic" {
		logger.Log.Info("Device already bound to uio_pci_generic", "pci", pci)
		return nil
	}

	if err == nil {
		unbindPath := filepath.Join(sysfsPath, "driver", "unbind")
		os.WriteFile(unbindPath, []byte(pci), 0200)
	}

	vendorID, _ := GetPCIVendorID(pci)
	deviceID, _ := GetPCIDeviceID(pci)

	newID := fmt.Sprintf("%s %s", vendorID, deviceID)
	os.WriteFile("/sys/bus/pci/drivers/uio_pci_generic/new_id", []byte(newID), 0200)
	os.WriteFile("/sys/bus/pci/drivers/uio_pci_generic/bind", []byte(pci), 0200)

	logger.Log.Info("Bound device to uio_pci_generic", "pci", pci)
	return nil
}
