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
	handlers.RegisterFactory(NewSetUserEnabledHandler)
}

type SetUserEnabledHandler struct {
	deps   *handlers.OperDeps
	logger *slog.Logger
}



func NewSetUserEnabledHandler(deps *handlers.OperDeps) handlers.OperHandler {
	return &SetUserEnabledHandler{
		deps:   deps,
		logger: logger.Component(local.Namespace + ".oper"),
	}
}

func (h *SetUserEnabledHandler) Execute(ctx context.Context, req *oper.Request) (interface{}, error) {
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

	var setReq local.SetUserEnabledRequest
	if err := json.Unmarshal(req.Body, &setReq); err != nil {
		return nil, fmt.Errorf("invalid request body: %w", err)
	}

	if err := local.UpdateUserEnabledByID(db, userID, setReq.Enabled); err != nil {
		return nil, fmt.Errorf("failed to set enabled: %w", err)
	}

	h.logger.Info("User enabled status updated", "user_id", userID, "enabled", setReq.Enabled)

	return &local.OperResponse{
		Message: fmt.Sprintf("User %d enabled status set to %v", userID, setReq.Enabled),
	}, nil
}

func (h *SetUserEnabledHandler) PathPattern() operpaths.Path {
	return operpaths.Path(local.OperSetUserEnabledPath)
}

func (h *SetUserEnabledHandler) Dependencies() []operpaths.Path {
	return nil
}
