package user

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/oper"
	"github.com/veesix-networks/osvbng/pkg/oper/handlers"
	operpaths "github.com/veesix-networks/osvbng/pkg/oper/paths"
	"github.com/veesix-networks/osvbng/plugins/auth/local"
)

func init() {
	handlers.RegisterFactory(NewSetUserPasswordHandler)
}

type SetUserPasswordHandler struct {
	deps   *handlers.OperDeps
	logger *slog.Logger
}



func NewSetUserPasswordHandler(deps *handlers.OperDeps) handlers.OperHandler {
	return &SetUserPasswordHandler{
		deps:   deps,
		logger: logger.Component(local.Namespace + ".oper"),
	}
}

func (h *SetUserPasswordHandler) Execute(ctx context.Context, req *oper.Request) (interface{}, error) {
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

	var setReq local.SetUserPasswordRequest
	if err := json.Unmarshal(req.Body, &setReq); err != nil {
		return nil, fmt.Errorf("invalid request body: %w", err)
	}

	if setReq.Password == "" {
		return nil, fmt.Errorf("password is required")
	}

	if err := local.UpdateUserPasswordByID(db, userID, &setReq.Password); err != nil {
		return nil, fmt.Errorf("failed to set password: %w", err)
	}

	h.logger.Info("User password updated", "user_id", userID)

	return &local.OperResponse{
		Message: fmt.Sprintf("Password updated for user %d", userID),
	}, nil
}

func (h *SetUserPasswordHandler) PathPattern() operpaths.Path {
	return operpaths.Path(local.OperSetUserPasswordPath)
}

func (h *SetUserPasswordHandler) Dependencies() []operpaths.Path {
	return nil
}
