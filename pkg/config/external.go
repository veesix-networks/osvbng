package config

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"text/template"
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

	tmpl, err := template.New(templateName).Parse(string(tmplContent))
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
		emptyConfig := &Config{}
		if err := rc.WriteConfig(emptyConfig); err != nil {
			return fmt.Errorf("write routing config: %w", err)
		}
		log.Printf("Generated %s", rc.ConfigPath)
	} else {
		log.Printf("Skipping %s (already exists)", rc.ConfigPath)
	}

	return nil
}
