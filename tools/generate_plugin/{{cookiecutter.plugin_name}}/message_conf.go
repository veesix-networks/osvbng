package {{cookiecutter.plugin_name}}

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/veesix-networks/osvbng/pkg/conf/handlers"
	"github.com/veesix-networks/osvbng/pkg/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/logger"
)

func init() {
	handlers.RegisterFactory(NewMessageHandler)
}

type MessageHandler struct {
	deps   *handlers.ConfDeps
	logger *slog.Logger
}

func NewMessageHandler(deps *handlers.ConfDeps) handlers.Handler {
	return &MessageHandler{
		deps:   deps,
		logger: logger.Component("{{cookiecutter.plugin_namespace}}.conf"),
	}
}

func (h *MessageHandler) Validate(ctx context.Context, hctx *handlers.HandlerContext) error {
	message, ok := hctx.NewValue.(string)
	if !ok {
		return fmt.Errorf("message must be a string, got %T", hctx.NewValue)
	}

	if message == "" {
		return fmt.Errorf("message cannot be empty")
	}

	return nil
}

func (h *MessageHandler) Apply(ctx context.Context, hctx *handlers.HandlerContext) error {
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

func (h *MessageHandler) Rollback(ctx context.Context, hctx *handlers.HandlerContext) error {
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

func (h *MessageHandler) Callbacks() *handlers.Callbacks {
	return nil
}
