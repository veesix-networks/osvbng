package service

import (
	"github.com/veesix-networks/osvbng/pkg/deps"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/veesix-networks/osvbng/pkg/handlers/oper"
	operpaths "github.com/veesix-networks/osvbng/pkg/handlers/oper/paths"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/plugins/auth/local"
)

func init() {
	oper.RegisterFactory(NewSetUserServiceHandler)
}

type SetUserServiceHandler struct {
	deps   *deps.OperDeps
	logger *slog.Logger
}



func NewSetUserServiceHandler(deps *deps.OperDeps) oper.OperHandler {
	return &SetUserServiceHandler{
		deps:   deps,
		logger: logger.Component(local.Namespace + ".oper"),
	}
}

func (h *SetUserServiceHandler) Execute(ctx context.Context, req *oper.Request) (interface{}, error) {
	provider := local.GetProvider()
	if provider == nil {
		return nil, fmt.Errorf("local auth provider not initialized")
	}

	db := provider.GetDB()

	wildcards, err := h.PathPattern().ExtractWildcards(req.Path, 2)
	if err != nil {
		return nil, err
	}
	userIDStr := wildcards[0]
	service := wildcards[1]

	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID: %s", userIDStr)
	}

	var setReq local.SetUserServiceRequest
	if err := json.Unmarshal(req.Body, &setReq); err != nil {
		return nil, fmt.Errorf("invalid request body: %w", err)
	}

	if err := local.AssignUserServiceByID(db, userID, service, setReq.Priority); err != nil {
		return nil, fmt.Errorf("failed to set user service: %w", err)
	}

	h.logger.Info("User service assigned", "user_id", userID, "service", service, "priority", setReq.Priority)

	return &local.OperResponse{
		Message: fmt.Sprintf("Service '%s' assigned to user %d with priority %d", service, userID, setReq.Priority),
	}, nil
}

func (h *SetUserServiceHandler) PathPattern() operpaths.Path {
	return operpaths.Path(local.OperSetUserServicePath)
}

func (h *SetUserServiceHandler) Dependencies() []operpaths.Path {
	return nil
}
