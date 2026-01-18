package local

import (
	"github.com/veesix-networks/osvbng/pkg/auth"
	"github.com/veesix-networks/osvbng/pkg/configmgr"
)

const Namespace = "subscriber.auth.local"

type Config struct {
	DatabasePath string `json:"database_path" yaml:"database_path"`
	AllowAll     bool   `json:"allow_all" yaml:"allow_all"`
}

func init() {
	configmgr.RegisterPluginConfig(Namespace, Config{})
	auth.Register("local", New)
}
