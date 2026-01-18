package config

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

const (
	DefaultDataplaneConfigPath = "/etc/osvbng/dataplane.conf"
)

type DataplaneConf struct {
	external   *ExternalConfig
	ConfigPath string
}

type DataplaneTemplateData struct {
	MainCore    int
	WorkerCores string
	LogFile     string
	CLISocket   string
	APISocket   string
	PuntSocket  string
	UseDPDK     bool
	DPDK        *DPDKConfig
}

func NewDataplaneTemplateDataWithDefaults(cfg *Config, totalCores int) *DataplaneTemplateData {
	mainCore := 0
	workerCores := ""
	if totalCores > 1 {
		workerCores = fmt.Sprintf("1-%d", totalCores-1)
	}

	dpdk := cfg.Dataplane.DPDK
	if dpdk == nil || len(dpdk.Devices) == 0 {
		devices, err := DiscoverDPDKDevices()
		if err == nil && len(devices) > 0 {
			if dpdk == nil {
				dpdk = &DPDKConfig{}
			}
			dpdk.Devices = devices
		}
	}

	useDPDK := dpdk != nil && len(dpdk.Devices) > 0

	return &DataplaneTemplateData{
		MainCore:    mainCore,
		WorkerCores: workerCores,
		LogFile:     "/var/log/osvbng/dataplane.log",
		CLISocket:   "/run/osvbng/cli.sock",
		APISocket:   "/run/osvbng/dataplane_api.sock",
		PuntSocket:  "/run/osvbng/punt.sock",
		UseDPDK:     useDPDK,
		DPDK:        dpdk,
	}
}

func NewDataplaneConf() *DataplaneConf {
	return &DataplaneConf{
		external:   NewExternalConfig(),
		ConfigPath: DefaultDataplaneConfigPath,
	}
}

func DiscoverDPDKDevices() ([]DPDKDevice, error) {
	cmd := exec.Command("lspci", "-Dmm")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("lspci failed: %w", err)
	}

	var devices []DPDKDevice
	scanner := bufio.NewScanner(bytes.NewReader(output))

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.Contains(strings.ToLower(line), "ethernet") {
			continue
		}

		if strings.Contains(strings.ToLower(line), "virtio") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 1 {
			continue
		}

		pciAddr := strings.TrimSpace(fields[0])

		devices = append(devices, DPDKDevice{
			PCI: pciAddr,
		})
	}

	return devices, nil
}

func (c *DataplaneConf) Generate(data *DataplaneTemplateData) (string, error) {
	return c.external.Generate("dataplane.conf.tmpl", data)
}

func (c *DataplaneConf) Write(data *DataplaneTemplateData) error {
	return c.external.Write("dataplane.conf.tmpl", c.ConfigPath, data)
}
