package hello

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/configmgr"
	"github.com/veesix-networks/osvbng/pkg/logger"
)

type Component struct {
	*component.Base
	logger  *slog.Logger
	message string
}

func NewComponent(deps component.Dependencies) (component.Component, error) {
	pluginCfgRaw, ok := configmgr.GetPluginConfig(Namespace)
	if !ok {
		return nil, nil
	}

	pluginCfg, ok := pluginCfgRaw.(*Config)
	if !ok {
		return nil, fmt.Errorf("invalid config type for %s", Namespace)
	}

	if !pluginCfg.Enabled {
		return nil, nil
	}

	message := "Hello from example plugin!"
	if pluginCfg.Message != "" {
		message = pluginCfg.Message
	}

	return &Component{
		Base:    component.NewBase(Namespace),
		logger:  logger.Component(Namespace),
		message: message,
	}, nil
}

func (c *Component) Start(ctx context.Context) error {
	c.StartContext(ctx)
	c.logger.Info("Plugin started", "message", c.message)
	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.StopContext()
	c.logger.Info("Plugin stopped")
	return nil
}

func (c *Component) GetMessage() string {
	return c.message
}

func (c *Component) SetMessage(msg string) {
	c.message = msg
}
