package config

import (
	"strings"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/config/system"
)

func TestDataplaneConfTemplate_DPDKDeviceNaming(t *testing.T) {
	tests := []struct {
		name     string
		data     *DataplaneTemplateData
		expected string
	}{
		{
			name: "DPDK devices should be named eth1, eth2, etc",
			data: &DataplaneTemplateData{
				MainCore:    0,
				WorkerCores: "1-3",
				LogFile:     "/var/log/osvbng/dataplane.log",
				CLISocket:   "/run/osvbng/cli.sock",
				APISocket:   "/run/osvbng/dataplane_api.sock",
				PuntSocket:  "/run/osvbng/punt.sock",
				UseDPDK:     true,
				DPDK: &system.DPDKConfig{
					UIODriver: "vfio-pci",
					Devices: []system.DPDKDevice{
						{PCI: "0000:00:06.0"},
						{PCI: "0000:00:07.0"},
						{PCI: "0000:00:08.0", Name: "custom-name"},
					},
				},
			},
			expected: `unix {
  interactive
  log /var/log/osvbng/dataplane.log
  full-coredump
  cli-listen /run/osvbng/cli.sock
  cli-prompt osvbng#
  cli-no-pager
  poll-sleep-usec 100
}

socksvr {
  socket-name /run/osvbng/dataplane_api.sock
}

api-trace {
  on
}

memory {
  main-heap-size 512M
  main-heap-page-size 4k
}

api-segment {
  gid osvbng
}

cpu {
  main-core 0
  corelist-workers 1-3
}

buffers {
  buffers-per-numa 65536
  default data-size 2048
  page-size default-hugepage
}

plugins {
  plugin default { enable }
  plugin dpdk_plugin.so { enable }
  plugin linux_cp_plugin.so { enable }
  plugin linux_nl_plugin.so { enable }
  plugin arp_plugin.so { disable }
  plugin rd_cp_plugin.so { disable }
  plugin igmp_plugin.so { disable }
  plugin v6n_osvbng_arp_punt_plugin.so { enable }
  plugin v6n_osvbng_fib_control_plugin.so { enable }
  plugin v6n_osvbng_accounting_plugin.so { enable }
}

logging {
  default-log-level info
  default-syslog-log-level info
}

linux-cp {
  lcp-sync
  lcp-auto-subint
  del-static-on-link-down
  del-dynamic-on-link-down
}

punt {
  socket /run/osvbng/punt.sock
}

statseg {
  default
  per-node-counters on
}

dpdk {
  uio-driver vfio-pci
  dev 0000:00:06.0 {
    name eth1
  }
  dev 0000:00:07.0 {
    name eth2
  }
  dev 0000:00:08.0 {
    name custom-name
  }
}
`,
		},
		{
			name: "Single DPDK device should be eth1",
			data: &DataplaneTemplateData{
				MainCore:    0,
				WorkerCores: "",
				LogFile:     "/var/log/osvbng/dataplane.log",
				CLISocket:   "/run/osvbng/cli.sock",
				APISocket:   "/run/osvbng/dataplane_api.sock",
				PuntSocket:  "/run/osvbng/punt.sock",
				UseDPDK:     true,
				DPDK: &system.DPDKConfig{
					Devices: []system.DPDKDevice{
						{PCI: "0000:00:06.0"},
					},
				},
			},
			expected: `unix {
  interactive
  log /var/log/osvbng/dataplane.log
  full-coredump
  cli-listen /run/osvbng/cli.sock
  cli-prompt osvbng#
  cli-no-pager
  poll-sleep-usec 100
}

socksvr {
  socket-name /run/osvbng/dataplane_api.sock
}

api-trace {
  on
}

memory {
  main-heap-size 512M
  main-heap-page-size 4k
}

api-segment {
  gid osvbng
}

cpu {
  main-core 0
}

buffers {
  buffers-per-numa 65536
  default data-size 2048
  page-size default-hugepage
}

plugins {
  plugin default { enable }
  plugin dpdk_plugin.so { enable }
  plugin linux_cp_plugin.so { enable }
  plugin linux_nl_plugin.so { enable }
  plugin arp_plugin.so { disable }
  plugin rd_cp_plugin.so { disable }
  plugin igmp_plugin.so { disable }
  plugin v6n_osvbng_arp_punt_plugin.so { enable }
  plugin v6n_osvbng_fib_control_plugin.so { enable }
  plugin v6n_osvbng_accounting_plugin.so { enable }
}

logging {
  default-log-level info
  default-syslog-log-level info
}

linux-cp {
  lcp-sync
  lcp-auto-subint
  del-static-on-link-down
  del-dynamic-on-link-down
}

punt {
  socket /run/osvbng/punt.sock
}

statseg {
  default
  per-node-counters on
}

dpdk {
  dev 0000:00:06.0 {
    name eth1
  }
}
`,
		},
		{
			name: "No DPDK should not have dev sections",
			data: &DataplaneTemplateData{
				MainCore:    0,
				WorkerCores: "1",
				LogFile:     "/var/log/osvbng/dataplane.log",
				CLISocket:   "/run/osvbng/cli.sock",
				APISocket:   "/run/osvbng/dataplane_api.sock",
				PuntSocket:  "/run/osvbng/punt.sock",
				UseDPDK:     false,
				DPDK:        nil,
			},
			expected: `unix {
  interactive
  log /var/log/osvbng/dataplane.log
  full-coredump
  cli-listen /run/osvbng/cli.sock
  cli-prompt osvbng#
  cli-no-pager
  poll-sleep-usec 100
}

socksvr {
  socket-name /run/osvbng/dataplane_api.sock
}

api-trace {
  on
}

memory {
  main-heap-size 512M
  main-heap-page-size 4k
}

api-segment {
  gid osvbng
}

cpu {
  main-core 0
  corelist-workers 1
}

buffers {
  buffers-per-numa 65536
  default data-size 2048
  page-size 4k
}

plugins {
  plugin default { enable }
  plugin dpdk_plugin.so { disable }
  plugin linux_cp_plugin.so { enable }
  plugin linux_nl_plugin.so { enable }
  plugin arp_plugin.so { disable }
  plugin rd_cp_plugin.so { disable }
  plugin igmp_plugin.so { disable }
  plugin v6n_osvbng_arp_punt_plugin.so { enable }
  plugin v6n_osvbng_fib_control_plugin.so { enable }
  plugin v6n_osvbng_accounting_plugin.so { enable }
}

logging {
  default-log-level info
  default-syslog-log-level info
}

linux-cp {
  lcp-sync
  lcp-auto-subint
  del-static-on-link-down
  del-dynamic-on-link-down
}

punt {
  socket /run/osvbng/punt.sock
}

statseg {
  default
  per-node-counters on
}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := NewDataplaneConf()
			conf.external.TemplateDir = "../../templates"
			output, err := conf.Generate(tt.data)
			if err != nil {
				t.Fatalf("Generate() error = %v", err)
			}

			if output != tt.expected {
				t.Errorf("Generated output does not match expected.\n\nExpected:\n%s\n\nGot:\n%s", tt.expected, output)
			}
		})
	}
}

func TestDataplaneConfTemplate_ManagementInterfaceAvoidance(t *testing.T) {
	data := &DataplaneTemplateData{
		MainCore:    0,
		WorkerCores: "1-3",
		LogFile:     "/var/log/osvbng/dataplane.log",
		CLISocket:   "/run/osvbng/cli.sock",
		APISocket:   "/run/osvbng/dataplane_api.sock",
		PuntSocket:  "/run/osvbng/punt.sock",
		UseDPDK:     true,
		DPDK: &system.DPDKConfig{
			Devices: []system.DPDKDevice{
				{PCI: "0000:00:06.0"},
			},
		},
	}

	conf := NewDataplaneConf()
	conf.external.TemplateDir = "../../templates"
	output, err := conf.Generate(data)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if strings.Contains(output, "name eth0") {
		t.Errorf("DPDK devices should not be named eth0 (reserved for management)")
	}
}
