package attribute

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
	oper.RegisterFactory(NewSetUserAttributeHandler)
}

type SetUserAttributeHandler struct {
	deps   *deps.OperDeps
	logger *slog.Logger
}



func NewSetUserAttributeHandler(deps *deps.OperDeps) oper.OperHandler {
	return &SetUserAttributeHandler{
		deps:   deps,
		logger: logger.Get(local.Namespace).WithGroup("oper"),
	}
}

func (h *SetUserAttributeHandler) Execute(ctx context.Context, req *oper.Request) (interface{}, error) {
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

	var setReq local.SetUserAttributeRequest
	if err := json.Unmarshal(req.Body, &setReq); err != nil {
		return nil, fmt.Errorf("invalid request body: %w", err)
	}

	if setReq.Attribute == "" || setReq.Value == "" || setReq.Op == "" {
		return nil, fmt.Errorf("attribute, value and op are required")
	}

	if err := local.SetAttributeByID(db, local.EntityTypeUser, userID, local.AttributeTypeResponse, setReq.Attribute, setReq.Value, setReq.Op); err != nil {
		return nil, fmt.Errorf("failed to set user attribute: %w", err)
	}

	h.logger.Info("User attribute set", "user_id", userID, "attribute", setReq.Attribute, "value", setReq.Value, "op", setReq.Op)

	return &local.OperResponse{
		Message: fmt.Sprintf("Attribute '%s' set for user %d", setReq.Attribute, userID),
	}, nil
}

func (h *SetUserAttributeHandler) PathPattern() operpaths.Path {
	return operpaths.Path(local.OperSetUserAttributePath)
}

func (h *SetUserAttributeHandler) Dependencies() []operpaths.Path {
	return nil
}
