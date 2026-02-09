package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

const (
	DefaultRoutingDaemonsPath = "/etc/osvbng/routing-daemons"
	DefaultRoutingConfigPath  = "/etc/osvbng/frr.conf"
	DefaultRoutingLogFile     = "/var/log/osvbng/routing.log"
)

type RoutingConf struct {
	external    *ExternalConfig
	DaemonsPath string
	ConfigPath  string
}

type RoutingDaemonsData struct {
	LogFile string
}

func NewRoutingConf() *RoutingConf {
	return &RoutingConf{
		external:    NewExternalConfig(),
		DaemonsPath: DefaultRoutingDaemonsPath,
		ConfigPath:  DefaultRoutingConfigPath,
	}
}

func (r *RoutingConf) GenerateDaemons(data *RoutingDaemonsData) (string, error) {
	return r.external.Generate("routing-daemons.tmpl", data)
}

func (r *RoutingConf) WriteDaemons(data *RoutingDaemonsData) error {
	return r.external.Write("routing-daemons.tmpl", r.DaemonsPath, data)
}

func (r *RoutingConf) GenerateConfig(cfg *Config) (string, error) {
	subTemplates := filepath.Join(r.external.TemplateDir, "frr", "*.tmpl")
	masterPath := filepath.Join(r.external.TemplateDir, "frr.conf.tmpl")

	masterContent, err := os.ReadFile(masterPath)
	if err != nil {
		return "", fmt.Errorf("read master template: %w", err)
	}

	tmpl, err := template.New("frr.conf.tmpl").ParseGlob(subTemplates)
	if err != nil {
		return "", fmt.Errorf("parse sub-templates: %w", err)
	}

	tmpl, err = tmpl.Parse(string(masterContent))
	if err != nil {
		return "", fmt.Errorf("parse master template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, cfg); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}

func (r *RoutingConf) WriteConfig(cfg *Config) error {
	content, err := r.GenerateConfig(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(r.ConfigPath, []byte(content), 0644)
}
