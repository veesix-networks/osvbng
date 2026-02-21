package config

import (
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/system"
)

const (
	DefaultDataplaneConfigPath = "/etc/osvbng/dataplane.conf"
	LCPNetNs                   = "dataplane"
)

type DataplaneConf struct {
	external   *ExternalConfig
	ConfigPath string
}

type DataplaneTemplateData struct {
	MainCore     int
	WorkerCores  string
	LogFile      string
	CLISocket    string
	APISocket    string
	PuntSocket   string
	UseDPDK      bool
	DPDK         *system.DPDKConfig
	StatsSegment *system.StatsSegmentConfig
	LCPNetNs     string
}

func NewDataplaneTemplateDataWithDefaults(cfg *Config, totalCores int) *DataplaneTemplateData {
	mainCore := 0
	workerCores := ""
	if totalCores > 1 {
		workerCores = fmt.Sprintf("1-%d", totalCores-1)
	}

	dpdk := cfg.Dataplane.DPDK
	if dpdk == nil || len(dpdk.Devices) == 0 {
		devices, err := system.DiscoverDPDKDevices()
		if err == nil && len(devices) > 0 {
			if dpdk == nil {
				dpdk = &system.DPDKConfig{}
			}
			dpdk.Devices = devices
		}
	}

	useDPDK := dpdk != nil && len(dpdk.Devices) > 0

	statseg := cfg.Dataplane.StatsSegment
	if statseg == nil {
		statseg = &system.StatsSegmentConfig{
			Size:            "256m",
			PageSize:        "4k",
			PerNodeCounters: false,
		}
	}

	return &DataplaneTemplateData{
		MainCore:     mainCore,
		WorkerCores:  workerCores,
		LogFile:      "/var/log/osvbng/dataplane.log",
		CLISocket:    "/run/osvbng/cli.sock",
		APISocket:    "/run/osvbng/dataplane_api.sock",
		PuntSocket:   "/run/osvbng/punt.sock",
		UseDPDK:      useDPDK,
		DPDK:         dpdk,
		StatsSegment: statseg,
		LCPNetNs:     LCPNetNs,
	}
}

func NewDataplaneConf() *DataplaneConf {
	return &DataplaneConf{
		external:   NewExternalConfig(),
		ConfigPath: DefaultDataplaneConfigPath,
	}
}

func (c *DataplaneConf) Generate(data *DataplaneTemplateData) (string, error) {
	return c.external.Generate("dataplane.conf.tmpl", data)
}

func (c *DataplaneConf) Write(data *DataplaneTemplateData) error {
	return c.external.Write("dataplane.conf.tmpl", c.ConfigPath, data)
}
