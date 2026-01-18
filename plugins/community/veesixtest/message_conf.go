package veesixtest

import (
	"github.com/veesix-networks/osvbng/pkg/deps"
	"context"
	"fmt"
	"log/slog"

	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/logger"
)

func init() {
	conf.RegisterFactory(NewMessageHandler)
}

type MessageHandler struct {
	deps   *deps.ConfDeps
	logger *slog.Logger
}

func NewMessageHandler(deps *deps.ConfDeps) conf.Handler {
	return &MessageHandler{
		deps:   deps,
		logger: logger.Component("veesixtest.conf"),
	}
}

func (h *MessageHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	message, ok := hctx.NewValue.(string)
	if !ok {
		return fmt.Errorf("message must be a string, got %T", hctx.NewValue)
	}

	if message == "" {
		return fmt.Errorf("message cannot be empty")
	}

	return nil
}

func (h *MessageHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	message := hctx.NewValue.(string)

	comp, ok := h.deps.PluginComponents[Namespace]
	if !ok {
		return fmt.Errorf("%s component not loaded", Namespace)
	}

	pluginComp, ok := comp.(*Component)
	if !ok {
		return fmt.Errorf("invalid component type for %s", Namespace)
	}

	pluginComp.SetMessage(message)
	h.logger.Info("Updated message via config", "message", message)
	return nil
}

func (h *MessageHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	if hctx.OldValue == nil {
		return nil
	}

	oldMessage := hctx.OldValue.(string)

	comp, ok := h.deps.PluginComponents[Namespace]
	if !ok {
		return fmt.Errorf("%s component not loaded", Namespace)
	}

	pluginComp, ok := comp.(*Component)
	if !ok {
		return fmt.Errorf("invalid component type for %s", Namespace)
	}

	pluginComp.SetMessage(oldMessage)
	return nil
}

func (h *MessageHandler) PathPattern() paths.Path {
	return paths.Path(ConfMessagePath)
}

func (h *MessageHandler) Dependencies() []paths.Path {
	return nil
}

func (h *MessageHandler) Callbacks() *conf.Callbacks {
	return nil
}
