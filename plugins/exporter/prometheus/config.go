package prometheus

import (
	"fmt"

	osvbngconfig "github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/configmgr"
	"github.com/veesix-networks/osvbng/pkg/netbind"
)

const Namespace = "exporter.prometheus"

type Config struct {
	netbind.ListenerBinding `json:",inline" yaml:",inline"`
	Enabled                 bool                    `yaml:"enabled" json:"enabled"`
	ListenAddress           string                  `yaml:"listen_address" json:"listen_address"`
	TLS                     netbind.ServerTLSConfig `json:"tls,omitempty" yaml:"tls,omitempty"`
}

func init() {
	configmgr.RegisterPluginConfig(Namespace, Config{})
	configmgr.RegisterPostVRFValidator(Namespace, validateBinding)
}

func validateBinding(cfg *osvbngconfig.Config, vrfMgr netbind.VRFResolver, nl netbind.LinkLister) error {
	pluginCfg, err := configmgr.DecodeCandidatePluginConfig[Config](cfg, Namespace)
	if err != nil {
		return fmt.Errorf("%s: %w", Namespace, err)
	}
	if pluginCfg == nil {
		return nil
	}
	if err := pluginCfg.ListenerBinding.Validate(netbind.FamilyV4, vrfMgr, nl); err != nil {
		return err
	}
	return pluginCfg.TLS.Validate()
}
