package configmgr

import (
	"encoding/json"

	"github.com/veesix-networks/osvbng/pkg/handlers/conf/types"
)

func (cd *ConfigManager) deepCopyConfig(src *types.Config) *types.Config {
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
