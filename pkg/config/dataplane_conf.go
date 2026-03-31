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
	Memory       *system.MemoryConfig
	APITrace     *system.APITraceConfig
	LCPNetNs     string
}

func NewDataplaneTemplateData(cfg *Config, cpu *ResolvedCPU) (*DataplaneTemplateData, error) {
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

	memory := cfg.Dataplane.Memory
	if memory == nil {
		memory = &system.MemoryConfig{
			MainHeapSize:     "1G",
			MainHeapPageSize: "4k",
		}
	}
	if memory.MainHeapSize == "" {
		memory.MainHeapSize = "1G"
	}
	if memory.MainHeapPageSize == "" {
		memory.MainHeapPageSize = "4k"
	}

	if err := memory.Validate(); err != nil {
		return nil, fmt.Errorf("dataplane memory config: %w", err)
	}

	apiTrace := cfg.Dataplane.APITrace

	return &DataplaneTemplateData{
		MainCore:     cpu.MainCore,
		WorkerCores:  cpu.WorkerCores,
		LogFile:      "/var/log/osvbng/dataplane.log",
		CLISocket:    "/run/osvbng/cli.sock",
		APISocket:    "/run/osvbng/dataplane_api.sock",
		PuntSocket:   "/run/osvbng/punt.sock",
		UseDPDK:      useDPDK,
		DPDK:         dpdk,
		StatsSegment: statseg,
		Memory:       memory,
		APITrace:     apiTrace,
		LCPNetNs:     lcpNetNs(cfg),
	}, nil
}

func lcpNetNs(cfg *Config) string {
	if cfg.Dataplane.LCPNetNs != "" {
		return cfg.Dataplane.LCPNetNs
	}
	return LCPNetNs
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
