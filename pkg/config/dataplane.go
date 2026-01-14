package config

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

const (
	DefaultDataplaneTemplateDir = "/usr/share/osvbng/templates"
	DefaultDataplaneConfigPath  = "/etc/osvbng/dataplane.conf"
)

type DataplaneConf struct {
	TemplateDir string
	ConfigPath  string
}

type DataplaneTemplateData struct {
	MainCore    int
	WorkerCores string
	LogFile     string
	CLISocket   string
	APISocket   string
	PuntSocket  string
	UseDPDK     bool
	DPDK        *DPDK
}

func NewDataplaneConf() *DataplaneConf {
	return &DataplaneConf{
		TemplateDir: DefaultDataplaneTemplateDir,
		ConfigPath:  DefaultDataplaneConfigPath,
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
	templatePath := filepath.Join(c.TemplateDir, "dataplane.conf.tmpl")

	tmplContent, err := os.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("read template: %w", err)
	}

	tmpl, err := template.New("dataplane").Parse(string(tmplContent))
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}

func (c *DataplaneConf) Write(data *DataplaneTemplateData) error {
	content, err := c.Generate(data)
	if err != nil {
		return err
	}

	return os.WriteFile(c.ConfigPath, []byte(content), 0644)
}
