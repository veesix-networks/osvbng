package frr

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/veesix-networks/osvbng/pkg/handlers/conf/types"
)

const (
	DefaultTemplateDir = "/usr/share/osvbng/templates"
	DefaultConfigPath  = "/etc/osvbng/routing.conf"
	DefaultReloadCmd   = "/usr/lib/frr/frr-reload.py"
)

type Config struct {
	TemplateDir string
	ConfigPath  string
	ReloadCmd   string
}

func NewConfig() *Config {
	return &Config{
		TemplateDir: DefaultTemplateDir,
		ConfigPath:  DefaultConfigPath,
		ReloadCmd:   DefaultReloadCmd,
	}
}

func (c *Config) GenerateConfig(config *types.Config) (string, error) {
	templatePath := filepath.Join(c.TemplateDir, "frr.conf.tmpl")

	tmplContent, err := os.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("read template: %w", err)
	}

	tmpl, err := template.New("frr").Parse(string(tmplContent))
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, config); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}

func (c *Config) Test(config *types.Config) error {
	candidateConfig, err := c.GenerateConfig(config)
	if err != nil {
		return fmt.Errorf("generate config: %w", err)
	}

	candidateFile, err := os.CreateTemp("", "frr-candidate-*.conf")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(candidateFile.Name())

	if _, err := candidateFile.WriteString(candidateConfig); err != nil {
		candidateFile.Close()
		return fmt.Errorf("write candidate config: %w", err)
	}
	candidateFile.Close()

	cmd := exec.Command(c.ReloadCmd, "--test", candidateFile.Name())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("frr config validation failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

func (c *Config) GetRunningConfig() (string, error) {
	cmd := exec.Command("vtysh", "-c", "show running-config")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to obtain routing CP configuration: %w\nOutput: %s", err, string(output))
	}

	return string(output), nil
}

func (c *Config) Reload(config *types.Config) error {
	candidateConfig, err := c.GenerateConfig(config)
	if err != nil {
		return fmt.Errorf("generate config: %w", err)
	}

	candidateFile, err := os.CreateTemp("", "frr-candidate-*.conf")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(candidateFile.Name())

	if _, err := candidateFile.WriteString(candidateConfig); err != nil {
		candidateFile.Close()
		return fmt.Errorf("write candidate config: %w", err)
	}
	candidateFile.Close()

	cmd := exec.Command(c.ReloadCmd, "--reload", candidateFile.Name())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("frr-reload failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}
