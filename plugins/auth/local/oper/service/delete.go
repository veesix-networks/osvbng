package service

import (
	"github.com/veesix-networks/osvbng/pkg/deps"
	"context"
	"fmt"
	"log/slog"

	"github.com/veesix-networks/osvbng/pkg/handlers/oper"
	operpaths "github.com/veesix-networks/osvbng/pkg/handlers/oper/paths"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/plugins/auth/local"
)

func init() {
	oper.RegisterFactory(NewDeleteServiceHandler)
}

type DeleteServiceHandler struct {
	deps   *deps.OperDeps
	logger *slog.Logger
}

func NewDeleteServiceHandler(deps *deps.OperDeps) oper.OperHandler {
	return &DeleteServiceHandler{
		deps:   deps,
		logger: logger.Component(local.Namespace + ".oper"),
	}
}

func (h *DeleteServiceHandler) Execute(ctx context.Context, req *oper.Request) (interface{}, error) {
	provider := local.GetProvider()
	if provider == nil {
		return nil, fmt.Errorf("local auth provider not initialized")
	}

	db := provider.GetDB()

	wildcards, err := h.PathPattern().ExtractWildcards(req.Path, 1)
	if err != nil {
		return nil, err
	}
	serviceName := wildcards[0]

	if err := local.DeleteService(db, serviceName); err != nil {
		return nil, fmt.Errorf("failed to delete service: %w", err)
	}

	h.logger.Info("Service deleted", "name", serviceName)

	return &local.OperResponse{
		Message: fmt.Sprintf("Service '%s' deleted successfully", serviceName),
	}, nil
}

func (h *DeleteServiceHandler) PathPattern() operpaths.Path {
	return operpaths.Path(local.OperDeleteServicePath)
}

func (h *DeleteServiceHandler) Dependencies() []operpaths.Path {
	return nil
}
