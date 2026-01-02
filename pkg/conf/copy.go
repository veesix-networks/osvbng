package conf

import (
	"encoding/json"

	"github.com/veesix-networks/osvbng/pkg/conf/types"
)

func (cd *ConfigDaemon) deepCopyConfig(src *types.Config) *types.Config {
	if src == nil {
		return nil
	}

	data, err := json.Marshal(src)
	if err != nil {
		panic(err)
	}

	dst := &types.Config{}
	if err := json.Unmarshal(data, dst); err != nil {
		panic(err)
	}

	return dst
}
