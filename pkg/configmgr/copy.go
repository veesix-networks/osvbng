package configmgr

import (
	"encoding/json"

	"github.com/veesix-networks/osvbng/pkg/config"
)

func (cd *ConfigManager) deepCopyConfig(src *config.Config) *config.Config {
	if src == nil {
		return nil
	}

	data, err := json.Marshal(src)
	if err != nil {
		panic(err)
	}

	dst := &config.Config{}
	if err := json.Unmarshal(data, dst); err != nil {
		panic(err)
	}

	return dst
}
