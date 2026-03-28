package system

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/config/system/nic"
)

type DataplaneConfig struct {
	DPAPISocket     string              `json:"dp_api_socket,omitempty" yaml:"dp_api_socket,omitempty"`
	PuntSocketPath  string              `json:"punt_socket_path,omitempty" yaml:"punt_socket_path,omitempty"`
	MemifSocketPath string              `json:"memif_socket_path,omitempty" yaml:"memif_socket_path,omitempty"`
	RxMode          string              `json:"rx_mode,omitempty" yaml:"rx_mode,omitempty"`
	MainCore        *int                `json:"main_core,omitempty" yaml:"main-core,omitempty"`
	Workers         string              `json:"workers,omitempty" yaml:"workers,omitempty"`
	SkipConfGen     bool                `json:"skip_conf_gen,omitempty" yaml:"skip-conf-gen,omitempty"`
	LCPNetNs        string              `json:"lcp_netns,omitempty" yaml:"lcp-netns,omitempty"`
	DPDK            *DPDKConfig         `json:"dpdk,omitempty" yaml:"dpdk,omitempty"`
	StatsSegment    *StatsSegmentConfig `json:"statseg,omitempty" yaml:"statseg,omitempty"`
	Memory          *MemoryConfig       `json:"memory,omitempty" yaml:"memory,omitempty"`
	APITrace        *APITraceConfig     `json:"api-trace,omitempty" yaml:"api-trace,omitempty"`
}

var validHeapPageSizes = map[string]bool{
	"4k": true, "2m": true, "1g": true,
}

type MemoryConfig struct {
	MainHeapSize     string `json:"main-heap-size,omitempty" yaml:"main-heap-size,omitempty"`
	MainHeapPageSize string `json:"main-heap-page-size,omitempty" yaml:"main-heap-page-size,omitempty"`
}

func (m *MemoryConfig) Validate() error {
	if m.MainHeapSize != "" {
		s := strings.ToUpper(m.MainHeapSize)
		valid := false
		for _, suffix := range []string{"K", "M", "G"} {
			if strings.HasSuffix(s, suffix) {
				numPart := strings.TrimSuffix(s, suffix)
				if _, err := strconv.Atoi(numPart); err == nil {
					valid = true
				}
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid main-heap-size %q: must be a number followed by K, M, or G (e.g. 512M, 1G)", m.MainHeapSize)
		}
	}
	if m.MainHeapPageSize != "" {
		if !validHeapPageSizes[strings.ToLower(m.MainHeapPageSize)] {
			return fmt.Errorf("invalid main-heap-page-size %q: must be one of 4k, 2m, 1g", m.MainHeapPageSize)
		}
	}
	return nil
}

type APITraceConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
}

type StatsSegmentConfig struct {
	Size            string `json:"size,omitempty" yaml:"size,omitempty"`
	PageSize        string `json:"page-size,omitempty" yaml:"page-size,omitempty"`
	PerNodeCounters bool   `json:"per-node-counters,omitempty" yaml:"per-node-counters,omitempty"`
}

type DPDKConfig struct {
	UIODriver            string             `json:"uio_driver,omitempty" yaml:"uio_driver,omitempty"`
	Devices              []DPDKDevice       `json:"devices,omitempty" yaml:"devices,omitempty"`
	DevDefaults          *DPDKDeviceOptions `json:"dev_defaults,omitempty" yaml:"dev_defaults,omitempty"`
	SocketMem            string             `json:"socket_mem,omitempty" yaml:"socket_mem,omitempty"`
	NoMultiSeg           bool               `json:"no_multi_seg,omitempty" yaml:"no_multi_seg,omitempty"`
	NoTxChecksumOffload  bool               `json:"no_tx_checksum_offload,omitempty" yaml:"no_tx_checksum_offload,omitempty"`
	EnableTcpUdpChecksum bool               `json:"enable_tcp_udp_checksum,omitempty" yaml:"enable_tcp_udp_checksum,omitempty"`
	MaxSimdBitwidth      int                `json:"max_simd_bitwidth,omitempty" yaml:"max_simd_bitwidth,omitempty"`
}

type DPDKDevice struct {
	PCI     string             `json:"pci" yaml:"pci"`
	Name    string             `json:"name,omitempty" yaml:"name,omitempty"`
	Options *DPDKDeviceOptions `json:"options,omitempty" yaml:"options,omitempty"`
}

type DPDKDeviceOptions struct {
	NumRxQueues   int    `json:"num_rx_queues,omitempty" yaml:"num_rx_queues,omitempty"`
	NumTxQueues   int    `json:"num_tx_queues,omitempty" yaml:"num_tx_queues,omitempty"`
	NumRxDesc     int    `json:"num_rx_desc,omitempty" yaml:"num_rx_desc,omitempty"`
	NumTxDesc     int    `json:"num_tx_desc,omitempty" yaml:"num_tx_desc,omitempty"`
	TSO           bool   `json:"tso,omitempty" yaml:"tso,omitempty"`
	Devargs       string `json:"devargs,omitempty" yaml:"devargs,omitempty"`
	RssQueues     string `json:"rss_queues,omitempty" yaml:"rss_queues,omitempty"`
	NoRxInterrupt bool   `json:"no_rx_interrupt,omitempty" yaml:"no_rx_interrupt,omitempty"`
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

func BindDPDKDevices(devices []DPDKDevice) error {
	nicDevices := make([]nic.Device, len(devices))
	for i, dev := range devices {
		nicDevices[i] = nic.Device{
			PCI:  dev.PCI,
			Name: dev.Name,
		}
	}
	return nic.BindDevices(nicDevices)
}
