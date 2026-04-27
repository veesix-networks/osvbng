package api

import (
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/component"
	osvbngconfig "github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/configmgr"
	"github.com/veesix-networks/osvbng/pkg/netbind"
)

const Namespace = "northbound.api"

type Config struct {
	netbind.ListenerBinding `json:",inline" yaml:",inline"`
	Enabled                 bool                   `json:"enabled" yaml:"enabled"`
	ListenAddress           string                 `json:"listen_address,omitempty" yaml:"listen_address,omitempty"`
	TLS                     netbind.ServerTLSConfig `json:"tls,omitempty" yaml:"tls,omitempty"`
}

func init() {
	configmgr.RegisterPluginConfig(Namespace, Config{})

	component.Register(Namespace, NewComponent,
		component.WithAuthor("Veesix Networks"),
		component.WithVersion("1.0.0"),
	)

	configmgr.RegisterPostVRFValidator(Namespace, validateBinding)
}

func validateBinding(cfg *osvbngconfig.Config, vrfMgr netbind.VRFResolver, nl netbind.LinkLister) error {
	pluginCfg, err := configmgr.DecodeCandidatePluginConfig[Config](cfg, Namespace)
	if err != nil {
		return fmt.Errorf("northbound.api: %w", err)
	}
	if pluginCfg == nil {
		return nil
	}
	if err := pluginCfg.ListenerBinding.Validate(netbind.FamilyV4, vrfMgr, nl); err != nil {
		return err
	}
	return pluginCfg.TLS.Validate()
}
