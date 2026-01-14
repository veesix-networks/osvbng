package user

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper"
	operpaths "github.com/veesix-networks/osvbng/pkg/handlers/oper/paths"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/plugins/auth/local"
)

func init() {
	oper.RegisterFactory(NewCreateUserHandler)
}

type CreateUserHandler struct {
	deps   *deps.OperDeps
	logger *slog.Logger
}


func NewCreateUserHandler(deps *deps.OperDeps) oper.OperHandler {
	return &CreateUserHandler{
		deps:   deps,
		logger: logger.Component(local.Namespace + ".oper"),
	}
}

func (h *CreateUserHandler) Execute(ctx context.Context, req *oper.Request) (interface{}, error) {
	provider := local.GetProvider()
	if provider == nil {
		return nil, fmt.Errorf("local auth provider not initialized")
	}

	db := provider.GetDB()

	var createReq local.CreateUserRequest
	if err := json.Unmarshal(req.Body, &createReq); err != nil {
		return nil, fmt.Errorf("invalid request body: %w", err)
	}

	if createReq.Username == "" {
		return nil, fmt.Errorf("username is required")
	}

	userID, err := local.CreateUser(db, createReq.Username, createReq.Password, createReq.Enabled)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	user, err := local.GetUserByID(db, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get created user: %w", err)
	}

	h.logger.Info("User created", "username", createReq.Username, "user_id", user.ID)

	return &local.CreateUserResponse{
		UserID:   user.ID,
		Username: user.Username,
		Message:  "User created successfully",
	}, nil
}

func (h *CreateUserHandler) PathPattern() operpaths.Path {
	return operpaths.Path(local.OperCreateUserPath)
}

func (h *CreateUserHandler) Dependencies() []operpaths.Path {
	return nil
}
