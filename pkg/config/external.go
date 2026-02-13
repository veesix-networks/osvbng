package config

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"text/template"

	"github.com/veesix-networks/osvbng/pkg/config/system"
)

const (
	DefaultExternalTemplateDir = "/usr/share/osvbng/templates"
)

type ExternalConfig struct {
	TemplateDir string
}

func NewExternalConfig() *ExternalConfig {
	return &ExternalConfig{
		TemplateDir: DefaultExternalTemplateDir,
	}
}

func (e *ExternalConfig) Generate(templateName string, data interface{}) (string, error) {
	templatePath := filepath.Join(e.TemplateDir, templateName)

	tmplContent, err := os.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("read template %s: %w", templateName, err)
	}

	tmpl, err := template.New(templateName).Funcs(template.FuncMap{
		"add": func(a, b int) int { return a + b },
	}).Parse(string(tmplContent))
	if err != nil {
		return "", fmt.Errorf("parse template %s: %w", templateName, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template %s: %w", templateName, err)
	}

	return buf.String(), nil
}

func (e *ExternalConfig) Write(templateName, outputPath string, data interface{}) error {
	content, err := e.Generate(templateName, data)
	if err != nil {
		return err
	}

	return os.WriteFile(outputPath, []byte(content), 0644)
}

func GenerateExternalConfigs(configPath string) error {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Printf("Config file not found at %s, generating default config", configPath)
		defaultCfg, err := Generate(GenerateOptions{AllDataplane: true})
		if err != nil {
			return fmt.Errorf("generate default config: %w", err)
		}
		if err := os.WriteFile(configPath, []byte(defaultCfg), 0644); err != nil {
			return fmt.Errorf("write default config: %w", err)
		}
		log.Printf("Default config written to %s", configPath)
	}

	cfg, err := Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	configUpdated := false
	if cfg.Dataplane.DPDK == nil || len(cfg.Dataplane.DPDK.Devices) == 0 {
		devices, err := system.DiscoverDPDKDevices()
		if err != nil {
			log.Printf("Warning: DPDK device discovery failed: %v", err)
		} else if len(devices) > 0 {
			for i := range devices {
				devices[i].Name = fmt.Sprintf("eth%d", i+1)
			}
			if cfg.Dataplane.DPDK == nil {
				cfg.Dataplane.DPDK = &system.DPDKConfig{}
			}
			cfg.Dataplane.DPDK.Devices = devices
			configUpdated = true
			log.Printf("Discovered %d DPDK devices", len(devices))
		}
	}

	if configUpdated {
		if err := Save(configPath, cfg); err != nil {
			return fmt.Errorf("save config with discovered DPDK devices: %w", err)
		}
		log.Printf("Updated %s with discovered DPDK devices", configPath)
	}

	if cfg.Dataplane.DPDK != nil && len(cfg.Dataplane.DPDK.Devices) > 0 {
		if err := system.BindDPDKDevices(cfg.Dataplane.DPDK.Devices); err != nil {
			log.Printf("Warning: DPDK device binding failed: %v", err)
		}
	}

	dc := NewDataplaneConf()
	if _, err := os.Stat(dc.ConfigPath); os.IsNotExist(err) {
		dpData := NewDataplaneTemplateDataWithDefaults(cfg, runtime.NumCPU())
		if err := dc.Write(dpData); err != nil {
			return fmt.Errorf("write dataplane config: %w", err)
		}
		log.Printf("Generated %s", dc.ConfigPath)
	} else {
		log.Printf("Skipping %s (already exists)", dc.ConfigPath)
	}

	rc := NewRoutingConf()
	if _, err := os.Stat(rc.DaemonsPath); os.IsNotExist(err) {
		if err := rc.WriteDaemons(&RoutingDaemonsData{
			LogFile: DefaultRoutingLogFile,
		}); err != nil {
			return fmt.Errorf("write routing daemons: %w", err)
		}
		log.Printf("Generated %s", rc.DaemonsPath)
	} else {
		log.Printf("Skipping %s (already exists)", rc.DaemonsPath)
	}

	if _, err := os.Stat(rc.ConfigPath); os.IsNotExist(err) {
		if err := rc.WriteConfig(cfg); err != nil {
			return fmt.Errorf("write routing config: %w", err)
		}
		log.Printf("Generated %s", rc.ConfigPath)
	} else {
		log.Printf("Skipping %s (already exists)", rc.ConfigPath)
	}

	return nil
}
