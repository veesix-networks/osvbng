package user

import (
	"github.com/veesix-networks/osvbng/pkg/deps"
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/veesix-networks/osvbng/pkg/handlers/oper"
	operpaths "github.com/veesix-networks/osvbng/pkg/handlers/oper/paths"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/plugins/auth/local"
)

func init() {
	oper.RegisterFactory(NewDeleteUserHandler)
}

type DeleteUserHandler struct {
	deps   *deps.OperDeps
	logger *slog.Logger
}


func NewDeleteUserHandler(deps *deps.OperDeps) oper.OperHandler {
	return &DeleteUserHandler{
		deps:   deps,
		logger: logger.Component(local.Namespace + ".oper"),
	}
}

func (h *DeleteUserHandler) Execute(ctx context.Context, req *oper.Request) (interface{}, error) {
	provider := local.GetProvider()
	if provider == nil {
		return nil, fmt.Errorf("local auth provider not initialized")
	}

	db := provider.GetDB()

	wildcards, err := h.PathPattern().ExtractWildcards(req.Path, 1)
	if err != nil {
		return nil, err
	}
	userIDStr := wildcards[0]

	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID: %s", userIDStr)
	}

	if err := local.DeleteUserByID(db, userID); err != nil {
		return nil, fmt.Errorf("failed to delete user: %w", err)
	}

	h.logger.Info("User deleted", "user_id", userID)

	return &local.OperResponse{
		Message: fmt.Sprintf("User %d deleted successfully", userID),
	}, nil
}

func (h *DeleteUserHandler) PathPattern() operpaths.Path {
	return operpaths.Path(local.OperDeleteUserPath)
}

func (h *DeleteUserHandler) Dependencies() []operpaths.Path {
	return nil
}
