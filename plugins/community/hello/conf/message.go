package conf

import (
	"github.com/veesix-networks/osvbng/pkg/deps"
	"context"
	"fmt"
	"log/slog"

	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/plugins/community/hello"
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
		logger: logger.Component("example.hello.conf"),
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

	comp, ok := h.deps.PluginComponents[hello.Namespace]
	if !ok {
		return fmt.Errorf("%s component not loaded", hello.Namespace)
	}

	helloComp, ok := comp.(*hello.Component)
	if !ok {
		return fmt.Errorf("invalid component type for %s", hello.Namespace)
	}

	helloComp.SetMessage(message)
	h.logger.Info("Updated message via config", "message", message)
	return nil
}

func (h *MessageHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	if hctx.OldValue == nil {
		return nil
	}

	oldMessage := hctx.OldValue.(string)

	comp, ok := h.deps.PluginComponents[hello.Namespace]
	if !ok {
		return fmt.Errorf("%s component not loaded", hello.Namespace)
	}

	helloComp, ok := comp.(*hello.Component)
	if !ok {
		return fmt.Errorf("invalid component type for %s", hello.Namespace)
	}

	helloComp.SetMessage(oldMessage)
	return nil
}

func (h *MessageHandler) PathPattern() paths.Path {
	return paths.Path(hello.ConfMessagePath)
}

func (h *MessageHandler) Dependencies() []paths.Path {
	return nil
}

func (h *MessageHandler) Callbacks() *conf.Callbacks {
	return nil
}
