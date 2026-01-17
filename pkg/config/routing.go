package config

import "github.com/veesix-networks/osvbng/pkg/handlers/conf/types"

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

func (r *RoutingConf) GenerateConfig(cfg *types.Config) (string, error) {
	return r.external.Generate("frr.conf.tmpl", cfg)
}

func (r *RoutingConf) WriteConfig(cfg *types.Config) error {
	return r.external.Write("frr.conf.tmpl", r.ConfigPath, cfg)
}
