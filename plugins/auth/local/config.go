package local

import (
	"github.com/veesix-networks/osvbng/pkg/auth"
	"github.com/veesix-networks/osvbng/pkg/conf"
)

const Namespace = "subscriber.auth.local"

type Config struct {
	DatabasePath string `json:"database_path" yaml:"database_path"`
}

func init() {
	conf.RegisterPluginConfig(Namespace, Config{})
	auth.Register("local", New)
}
